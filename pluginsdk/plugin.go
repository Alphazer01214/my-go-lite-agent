package pluginsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// Request is one inbound Host req delivered to a handler.
type Request struct {
	ID          string
	ContextID   string
	Capability  string
	Method      string
	Payload     json.RawMessage
	proceed     func(context.Context, json.RawMessage) (json.RawMessage, error)
	proceedOnce atomic.Bool
}

// Proceed continues a Frame Priority pipeline at most once.
func (r *Request) Proceed(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if r == nil || r.proceed == nil {
		return nil, ErrCode(CodeMethodNotFound, "request does not support proceed")
	}
	if !r.proceedOnce.CompareAndSwap(false, true) {
		return nil, ErrCode(CodeFlowAlreadyProceeded, "request already proceeded")
	}
	return r.proceed(ctx, payload)
}

type Event struct {
	ID         string
	ContextID  string
	Capability string
	Method     string
	Payload    json.RawMessage
}

// HandleFunc processes one inbound req and returns a result body (or error).
type HandleFunc func(ctx context.Context, req *Request) (json.RawMessage, error)

type outboundCall struct {
	id          string
	ch          chan *protocol.Frame
	contextID   string
	capability  string
	method      string
	cancelMu    sync.Mutex
	cancelCause error
}

func (c *outboundCall) markCancel(err error) bool {
	c.cancelMu.Lock()
	defer c.cancelMu.Unlock()
	if c.cancelCause != nil {
		return false
	}
	c.cancelCause = err
	return true
}

func (c *outboundCall) cancellationError() error {
	c.cancelMu.Lock()
	defer c.cancelMu.Unlock()
	if c.cancelCause == nil {
		return nil
	}
	return contextCallError(c.cancelCause)
}

type inboundCall struct {
	cancel context.CancelFunc
	reason string
}

// Handler wraps a HandleFunc with optional human-readable info (not used for routing).
type Handler struct {
	Name        string
	Description string
	handleFunc  HandleFunc
}

// NewHandler wraps fn so it can be passed to Register.
func NewHandler(fn HandleFunc) Handler {
	return Handler{handleFunc: fn}
}

// WithInfo returns a copy carrying Name/Description for listing and diagnostics.
func (h Handler) WithInfo(name, description string) Handler {
	h.Name = name
	h.Description = description
	return h
}

// Info returns the handler's optional name/description.
func (h Handler) Info() (name, description string) {
	return h.Name, h.Description
}

// Plugin is the in-process runtime: dispatches inbound req, completes outbound Call.
// Plugins never address by plugin name — only capability.method.
type Plugin struct {
	mu   sync.Mutex
	name string
	// handlers maps "capability.method" to a Handler
	handlers map[string]Handler
	// pending maps outbound Call id to its waiter state.
	// Close condition: res is delivered, or fail() on disconnect.
	pending map[string]*outboundCall
	// inbound tracks cancellable Handler contexts by request id.
	inbound map[string]*inboundCall
	stdin   *os.File
	stdout  *os.File

	seq atomic.Int32
}

// NewPlugin returns a Plugin bound to process stdio.
// name is identity only (logs / id prefix) — never used for routing.
func NewPlugin(name string) *Plugin {
	return &Plugin{
		name:     name,
		handlers: make(map[string]Handler),
		pending:  make(map[string]*outboundCall),
		inbound:  make(map[string]*inboundCall),
		stdin:    os.Stdin,
		stdout:   os.Stdout,
	}
}

// Register binds capability.method to handler.
// Same-plugin re-register is last-wins (protocol §4.2).
func (p *Plugin) Register(capability string, method string, handler Handler) {
	if capability == "" || method == "" {
		panic("Register: capability and method must be non-empty")
	}
	if handler.handleFunc == nil {
		panic("Register: handler must be built with NewHandler")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[capability+"."+method] = handler
}

// Registration is one registered (capability, method) → Handler entry.
type Registration struct {
	Capability string
	Method     string
	Handler    Handler
}

// ListRegistered returns a snapshot of registered handlers with their route keys.
func (p *Plugin) ListRegistered() []Registration {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Registration, 0, len(p.handlers))
	for key, h := range p.handlers {
		capability, method, _ := strings.Cut(key, ".")
		out = append(out, Registration{Capability: capability, Method: method, Handler: h})
	}
	return out
}

// Serve reports registered handlers to Host, then runs Listen until stdin closes.
// Typical main: NewPlugin → Register… →（可选）Call… → Serve().
func (p *Plugin) Serve() error {
	go p.reportRegistered()
	return p.Listen()
}

// reportRegistered is best-effort host.register (protocol §5.1).
// Failure is ignored — the route table is Host's concern; Listen still runs.
func (p *Plugin) reportRegistered() {
	regs := p.ListRegistered()
	handlers := make([]map[string]string, 0, len(regs))
	for _, r := range regs {
		handlers = append(handlers, map[string]string{
			"capability": r.Capability,
			"method":     r.Method,
		})
	}
	payload, _ := json.Marshal(map[string]any{"handlers": handlers})
	_, _ = p.Call(context.Background(), HostCapability, "register", payload)
}

// Listen is the receive loop: read stdin frames, dispatch req, complete pending res.
// Returns when stdin closes or a read fails; always runs fail() first.
func (p *Plugin) Listen() error {
	for {
		frame, err := protocol.ReadFrame(p.stdin)
		if err != nil {
			p.fail()
			return err
		}
		switch frame.Type {
		case protocol.FrameRequest:
			// handler may block; do not stall the receive loop
			go p.dispatch(frame)
		case protocol.FrameResponse:
			// ordered with evt so attributed stream stays before its res
			p.complete(frame)
		case protocol.FrameEvent:
			p.deliver(frame)
		case protocol.FrameCancel:
			p.handleCancel(frame)
		default:
			// unknown types are ignored (forward compatible)
		}
	}
}

// fail tears down every in-flight outbound Call and cancels inbound handlers.
func (p *Plugin) fail() {
	p.mu.Lock()
	calls := make([]*outboundCall, 0, len(p.pending))
	for id, call := range p.pending {
		calls = append(calls, call)
		delete(p.pending, id)
	}
	inbound := make([]*inboundCall, 0, len(p.inbound))
	for id, call := range p.inbound {
		inbound = append(inbound, call)
		delete(p.inbound, id)
	}
	p.mu.Unlock()
	for _, call := range calls {
		close(call.ch)
	}
	for _, call := range inbound {
		call.cancel()
	}
}

// complete delivers a res to the waiting Call, then closes the channel.
func (p *Plugin) complete(frame *protocol.Frame) {
	p.mu.Lock()
	call, ok := p.pending[frame.ID]
	if ok {
		delete(p.pending, frame.ID)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	call.ch <- frame
	close(call.ch)
}

// dispatch handles one inbound req and writes exactly one res (same id).
func (p *Plugin) dispatch(frame *protocol.Frame) {
	key := frame.Capability + "." + frame.Method
	p.mu.Lock()
	h, ok := p.handlers[key]
	p.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	entry := &inboundCall{cancel: cancel}
	p.mu.Lock()
	if existing := p.inbound[frame.ID]; existing != nil {
		entry = existing
		if existing.reason != "" {
			cancel()
		}
	} else {
		p.inbound[frame.ID] = entry
	}
	p.mu.Unlock()

	res := &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         frame.ID,
		Type:       protocol.FrameResponse,
		Capability: frame.Capability,
		Method:     frame.Method,
		Priority:   frame.Priority,
	}
	if !ok {
		res.ErrorCode = CodeMethodNotFound
		res.ErrorMsg = fmt.Sprintf("no handler for %s", key)
	} else {
		req := &Request{
			ID:         frame.ID,
			ContextID:  frame.ContextID,
			Capability: frame.Capability,
			Method:     frame.Method,
			Payload:    frame.Payload,
		}
		req.proceed = func(callCtx context.Context, payload json.RawMessage) (json.RawMessage, error) {
			body, marshalErr := json.Marshal(map[string]any{"call_id": frame.ID, "payload": payload})
			if marshalErr != nil {
				return nil, ErrCode(CodeBadPayload, marshalErr.Error())
			}
			return p.CallWithCallbackInContext(callCtx, frame.ContextID, HostCapability, "proceed", body, func(ev *Event) {
				_ = p.EmitWithID(frame.ID, ev.Capability, ev.Method, ev.Payload)
			})
		}
		out, err := h.handleFunc(ctx, req)
		if err != nil {
			code := Code(err)
			if code == "" {
				code = CodeHandlerError
			}
			res.ErrorCode = code
			res.ErrorMsg = err.Error()
		} else {
			res.Payload = out
		}
	}

	p.mu.Lock()
	entry = p.inbound[frame.ID]
	delete(p.inbound, frame.ID)
	p.mu.Unlock()
	cancel()
	if entry != nil && entry.reason != "" {
		res.Payload = nil
		res.ErrorCode = reasonCode(entry.reason)
		res.ErrorMsg = "call " + entry.reason
	}
	_ = p.writeFrame(res)
}

// handleCancel cancels one inbound Handler context and never writes a Response.
func (p *Plugin) handleCancel(frame *protocol.Frame) {
	p.mu.Lock()
	entry := p.inbound[frame.ID]
	if entry == nil {
		entry = &inboundCall{cancel: func() {}}
		p.inbound[frame.ID] = entry
	}
	if entry.reason == "" {
		entry.reason = normalizeCancelReason(frame.Reason)
	}
	cancel := entry.cancel
	p.mu.Unlock()
	cancel()
}

// deliver routes an attributed evt to the matching pending Call waiter.
// Unattributed evt (empty id) is dropped — it must not pollute Call channels.
func (p *Plugin) deliver(frame *protocol.Frame) {
	if frame.ID == "" {
		return
	}
	p.mu.Lock()
	call, ok := p.pending[frame.ID]
	p.mu.Unlock()
	if !ok {
		return
	}
	// Non-blocking: drop under backpressure so complete/fail cannot deadlock.
	select {
	case call.ch <- frame:
	default:
	}
}

func (p *Plugin) toEvent(frame *protocol.Frame) *Event {
	return &Event{
		ID:         frame.ID,
		ContextID:  frame.ContextID,
		Capability: frame.Capability,
		Method:     frame.Method,
		Payload:    frame.Payload,
	}
}

// waitResult drains the waiter until res (or channel close).
// Attributed evt frames go to callback when non-nil; otherwise they are discarded.
func (p *Plugin) waitResult(ctx context.Context, call *outboundCall, capability, method string, callback func(*Event)) (json.RawMessage, error) {
	for {
		select {
		case <-ctx.Done():
			if !p.markOutboundCancel(call, ctx.Err()) {
				continue
			}
			if err := p.writeCancel(call, ctx.Err()); err != nil {
				p.mu.Lock()
				delete(p.pending, call.id)
				p.mu.Unlock()
				return nil, contextCallError(ctx.Err())
			}
		case frame, ok := <-call.ch:
			if !ok || frame == nil {
				return nil, fmt.Errorf("call %s.%s: connection closed", capability, method)
			}
			switch frame.Type {
			case protocol.FrameEvent:
				if callback != nil {
					callback(p.toEvent(frame))
				}
			case protocol.FrameResponse:
				if canceled := call.cancellationError(); canceled != nil {
					return nil, canceled
				}
				if frame.ErrorCode != "" || frame.ErrorMsg != "" {
					code := frame.ErrorCode
					if code == "" {
						code = CodeHandlerError
					}
					return nil, ErrCode(code, frame.ErrorMsg)
				}
				return frame.Payload, nil
			default:
				// ignore unknown
			}
		}
	}
}

func contextCallError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCode(CodeCallDeadlineExceeded, "call deadline exceeded")
	}
	return ErrCode(CodeCallCancelled, "call cancelled")
}

// callPendingInContext registers one call in a Host execution context.
func (p *Plugin) callPendingInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (string, *outboundCall, error) {
	if ctx == nil {
		return "", nil, ErrContextRequired
	}
	if err := ctx.Err(); err != nil {
		return "", nil, contextCallError(err)
	}
	id := fmt.Sprintf("%s-%d", p.name, p.seq.Add(1))
	call := &outboundCall{
		id:         id,
		ch:         make(chan *protocol.Frame, 32),
		contextID:  contextID,
		capability: capability,
		method:     method,
	}

	p.mu.Lock()
	p.pending[id] = call
	p.mu.Unlock()

	// from is Host-injected on delivery; plugins leave it empty.
	frame := protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         id,
		Type:       protocol.FrameRequest,
		ContextID:  contextID,
		Capability: capability,
		Method:     method,
		Payload:    payload,
	}
	if err := p.writeFrame(&frame); err != nil {
		p.mu.Lock()
		_, ok := p.pending[id]
		delete(p.pending, id)
		p.mu.Unlock()
		if ok {
			close(call.ch)
		}
		return "", nil, fmt.Errorf("write call %s.%s: %w", capability, method, err)
	}
	return id, call, nil
}

// Call invokes capability.method through Host (star topology).
// The Go context controls waiting and is passed to nested Calls.
func (p *Plugin) Call(ctx context.Context, capability string, method string, payload json.RawMessage) (json.RawMessage, error) {
	_, call, err := p.callPendingInContext(ctx, "", capability, method, payload)
	if err != nil {
		return nil, err
	}
	return p.waitResult(ctx, call, capability, method, nil)
}

// CallWithCallback is Call plus attributed event delivery until the final res.
func (p *Plugin) CallWithCallback(ctx context.Context, capability, method string, payload json.RawMessage, callback func(event *Event)) (json.RawMessage, error) {
	return p.CallWithCallbackInContext(ctx, "", capability, method, payload, callback)
}

// CallInContext invokes capability.method inside one Host execution context.
func (p *Plugin) CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	_, call, err := p.callPendingInContext(ctx, contextID, capability, method, payload)
	if err != nil {
		return nil, err
	}
	return p.waitResult(ctx, call, capability, method, nil)
}

// CallWithCallbackInContext combines CallInContext with attributed event delivery.
func (p *Plugin) CallWithCallbackInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage, callback func(*Event)) (json.RawMessage, error) {
	_, call, err := p.callPendingInContext(ctx, contextID, capability, method, payload)
	if err != nil {
		return nil, err
	}
	return p.waitResult(ctx, call, capability, method, callback)
}

// Emit sends an unattributed evt Frame (no response expected).
func (p *Plugin) Emit(capability string, method string, payload json.RawMessage) error {
	return p.EmitInContext("", capability, method, payload)
}

// EmitInContext sends an unattributed event in one execution context.
func (p *Plugin) EmitInContext(contextID, capability, method string, payload json.RawMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return protocol.WriteFrame(p.stdout, &protocol.Frame{
		Version:    protocol.FrameVersion,
		Type:       protocol.FrameEvent,
		ContextID:  contextID,
		Capability: capability,
		Method:     method,
		Payload:    payload,
	})
}

// EmitWithID sends an attributed evt (id points at an in-flight Call).
func (p *Plugin) EmitWithID(id string, capability string, method string, payload json.RawMessage) error {
	return p.EmitWithIDInContext("", id, capability, method, payload)
}

// EmitWithIDInContext sends an attributed event in one execution context.
func (p *Plugin) EmitWithIDInContext(contextID, id string, capability string, method string, payload json.RawMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return protocol.WriteFrame(p.stdout, &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         id,
		ContextID:  contextID,
		Type:       protocol.FrameEvent,
		Capability: capability,
		Method:     method,
		Payload:    payload,
	})
}

func (p *Plugin) markOutboundCancel(call *outboundCall, cause error) bool {
	p.mu.Lock()
	current := p.pending[call.id]
	p.mu.Unlock()
	if current != call {
		return false
	}
	return call.markCancel(cause)
}

func (p *Plugin) writeCancel(call *outboundCall, cause error) error {
	return p.writeFrame(&protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         call.id,
		Type:       protocol.FrameCancel,
		ContextID:  call.contextID,
		Capability: call.capability,
		Method:     call.method,
		Reason:     normalizeCancelReason(cause.Error()),
	})
}

func normalizeCancelReason(reason string) string {
	if reason == context.DeadlineExceeded.Error() || reason == "deadline_exceeded" {
		return "deadline_exceeded"
	}
	return "cancelled"
}

func reasonCode(reason string) string {
	if reason == "deadline_exceeded" {
		return CodeCallDeadlineExceeded
	}
	return CodeCallCancelled
}

// writeFrame serializes one Frame onto stdout.
func (p *Plugin) writeFrame(frame *protocol.Frame) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return protocol.WriteFrame(p.stdout, frame)
}
