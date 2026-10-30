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
	ID         string
	From       string
	ContextID  string
	Capability string
	Method     string
	Payload    json.RawMessage
	proceed    func(context.Context, json.RawMessage) error
	// proceeded marks a successful handoff. It rejects a second Proceed of the
	// same request and stops this process's dispatch from writing a business
	// Response once the request left the handler.
	proceeded atomic.Bool
}

// Proceed hands the current request to the next hop of the Frame Priority
// pipeline. It is terminal: on success the request belongs to the next hop
// and the handler's result is not written back as a Response. On failure
// forwarding did not happen, so the handler may still return an error.
//
// A failed Proceed may be retried. The Host owns the one-handoff-per-call
// guarantee: it rejects a second handoff of the same call, so a retry can
// never consume the request twice.
func (r *Request) Proceed(ctx context.Context, payload json.RawMessage) error {
	if r == nil || r.proceed == nil {
		return ErrCode(CodeMethodNotFound, "request does not support proceed")
	}
	if r.proceeded.Load() {
		return ErrCode(CodeFlowAlreadyProceeded, "request already proceeded")
	}
	if err := r.proceed(ctx, payload); err != nil {
		return err
	}
	r.proceeded.Store(true)
	return nil
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
	// placeholder marks a cancel that arrived before its req was dispatched.
	// It carries no handler context and is dropped once the req lands or once
	// the id falls out of the bounded inbound window.
	placeholder bool
}

// Handler wraps a HandleFunc with optional human-readable info (not used for routing).
type Handler struct {
	Name        string
	Description string
	handleFunc  HandleFunc
}

// NewHandler wraps fn with optional handler metadata.
//
// Deprecated: Register accepts HandleFunc directly; keep this only for
// compatibility with callers that need Name or Description.
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

// inboundWindowSize bounds how many recent inbound frame ids the SDK remembers.
// It only exists to pair a cancel with a req that may arrive out of order, so a
// few hundred entries is far more than any in-flight window needs.
const inboundWindowSize = 256

// Plugin is the in-process runtime: dispatches inbound req, completes outbound Call.
// Plugins never address by plugin name — only capability.method.
type Plugin struct {
	mu sync.Mutex
	// writeMu serializes stdout frames without sharing the state lock. A
	// blocked pipe write must never stop Listen from completing pending calls.
	writeMu sync.Mutex
	name    string
	// handlers maps "capability.method" to a Handler
	handlers map[string]Handler
	// pending maps outbound Call id to its waiter state.
	// Close condition: res is delivered, or fail() on disconnect.
	pending map[string]*outboundCall
	// inbound tracks cancellable Handler contexts by request id.
	inbound map[string]*inboundCall
	// inboundWindow remembers the most recent inbound request ids, in arrival
	// order and bounded in size. A cancel may reach the SDK before the matching
	// req is dispatched, so handleCancel needs to tell "not started yet" from
	// "already finished" — without keeping every id forever.
	inboundRecent    []string
	inboundRecentSet map[string]bool
	stdin            *os.File
	stdout           *os.File

	seq atomic.Int32
}

// NewPlugin returns a Plugin bound to process stdio.
// name is identity only (logs / id prefix) — never used for routing.
func NewPlugin(name string) *Plugin {
	return &Plugin{
		name:             name,
		handlers:         make(map[string]Handler),
		pending:          make(map[string]*outboundCall),
		inbound:          make(map[string]*inboundCall),
		inboundRecentSet: make(map[string]bool),
		stdin:            os.Stdin,
		stdout:           os.Stdout,
	}
}

// Register binds capability.method to handler.
// Same-plugin re-register is last-wins (protocol §4.2).
func (p *Plugin) Register(capability string, method string, handler HandleFunc) {
	if capability == "" || method == "" {
		panic("Register: capability and method must be non-empty")
	}
	if handler == nil {
		panic("Register: handler must be non-nil")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[capability+"."+method] = Handler{handleFunc: handler}
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

// Serve reports the handlers registered so far to Host, then runs Listen until
// stdin closes. Typical main: NewPlugin → Register… →（可选）Call… → Serve().
// Handlers registered after Serve() starts are local only: re-announcing a route
// is Host's business and this runtime does not do it on its own.
func (p *Plugin) Serve() error {
	// Listen must already be draining before the register Call can complete,
	// so it runs on its own goroutine and Serve blocks on it instead.
	listenDone := make(chan error, 1)
	go func() { listenDone <- p.Listen() }()
	p.reportRegistered()
	return <-listenDone
}

// reportRegistered is best-effort host.register (protocol §5.1).
// Failure is ignored — the route table is Host's concern; Listen still runs.
func (p *Plugin) reportRegistered() {
	regs := p.ListRegistered()
	handlers := make([]map[string]string, 0, len(regs))
	for _, r := range regs {
		entry := map[string]string{
			"capability": r.Capability,
			"method":     r.Method,
		}
		if r.Handler.Name != "" {
			entry["name"] = r.Handler.Name
		}
		if r.Handler.Description != "" {
			entry["description"] = r.Handler.Description
		}
		handlers = append(handlers, entry)
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
			p.rememberInbound(frame.ID)
			go p.dispatch(frame)
		case protocol.FrameResponse:
			// ordered with evt so attributed stream stays before its res
			p.complete(frame)
		case protocol.FrameEvent:
			p.deliver(frame)
		case protocol.FrameCancel:
			p.rememberInbound(frame.ID)
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
	// Non-blocking: resCh is buffered for exactly this delivery. A full buffer
	// means the waiter already went away or a res was injected first; dropping
	// keeps the receive loop free instead of stalling every other frame.
	select {
	case call.ch <- frame:
	default:
	}
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
		entry.placeholder = false
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
	var req *Request
	if !ok {
		res.ErrorCode = CodeMethodNotFound
		res.ErrorMsg = fmt.Sprintf("no handler for %s", key)
	} else {
		req = &Request{
			ID:         frame.ID,
			From:       frame.From,
			ContextID:  frame.ContextID,
			Capability: frame.Capability,
			Method:     frame.Method,
			Payload:    frame.Payload,
		}
		req.proceed = func(callCtx context.Context, payload json.RawMessage) error {
			// An empty payload means "keep the request's payload"; encoding it
			// as JSON null would instead forward a literal null to the next hop.
			body := map[string]any{"call_id": frame.ID}
			if len(payload) != 0 {
				body["payload"] = payload
			}
			encoded, marshalErr := json.Marshal(body)
			if marshalErr != nil {
				return ErrCode(CodeBadPayload, marshalErr.Error())
			}
			// The ack only confirms the handoff; the terminal business
			// Response goes straight to the original caller.
			_, err := p.CallInContext(callCtx, frame.ContextID, HostCapability, "proceed", encoded)
			return err
		}
		out, err := h.handleFunc(ctx, req)
		if !req.proceeded.Load() {
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
	}

	p.mu.Lock()
	entry = p.inbound[frame.ID]
	delete(p.inbound, frame.ID)
	p.mu.Unlock()
	cancel()
	if req != nil && req.proceeded.Load() {
		// The request already left this handler; nothing owns a business res.
		return
	}
	if entry != nil && entry.reason != "" {
		res.Payload = nil
		res.ErrorCode = reasonCode(entry.reason)
		res.ErrorMsg = "call " + entry.reason
	}
	_ = p.writeFrame(res)
}

// handleCancel cancels one inbound Handler context and never writes a Response.
// Host forwards cancel and req over the same pipe without ordering guarantees, so
// a cancel may land first; that case parks the reason on a placeholder which the
// later dispatch adopts. Placeholders are bounded by inboundWindowSize.
func (p *Plugin) handleCancel(frame *protocol.Frame) {
	p.mu.Lock()
	entry := p.inbound[frame.ID]
	if entry == nil {
		entry = &inboundCall{cancel: func() {}, placeholder: true}
		p.inbound[frame.ID] = entry
	}
	if entry.reason == "" {
		entry.reason = normalizeCancelReason(frame.Reason)
	}
	cancel := entry.cancel
	p.mu.Unlock()
	cancel()
}

// rememberInbound opens a slot in the bounded inbound window for one frame id.
func (p *Plugin) rememberInbound(id string) {
	p.mu.Lock()
	p.rememberInboundLocked(id)
	p.mu.Unlock()
}

// rememberInboundLocked records one inbound frame id in the bounded window,
// evicting the oldest entries so neither the set nor the placeholders it backs
// can grow without limit.
func (p *Plugin) rememberInboundLocked(id string) {
	if p.inboundRecentSet[id] {
		return
	}
	p.inboundRecentSet[id] = true
	p.inboundRecent = append(p.inboundRecent, id)
	for len(p.inboundRecent) > inboundWindowSize {
		oldest := p.inboundRecent[0]
		p.inboundRecent = p.inboundRecent[1:]
		delete(p.inboundRecentSet, oldest)
		if entry := p.inbound[oldest]; entry != nil && entry.placeholder {
			delete(p.inbound, oldest)
		}
	}
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
// The caller's context bounds the wait directly: on cancellation the pending
// call is detached and a cancel Frame is sent best-effort. The waiter must not
// depend on that write, because the same blocked stdout can be the reason the
// context expired.
func (p *Plugin) waitResult(ctx context.Context, call *outboundCall, capability, method string, callback func(*Event)) (json.RawMessage, error) {
	for {
		select {
		case <-ctx.Done():
			// Return from the caller's context directly. Waiting for a cancel
			// frame to be written would make the timeout depend on the same
			// blocked pipe that caused the deadline.
			if p.markOutboundCancel(call, ctx.Err()) {
				go func() { _ = p.writeCancel(call, ctx.Err()) }()
			}
			p.removePending(call)
			return nil, contextCallError(ctx.Err())
		case frame, ok := <-call.ch:
			if !ok || frame == nil {
				if canceled := call.cancellationError(); canceled != nil {
					return nil, canceled
				}
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

// removePending detaches one outbound call without touching a different call
// that may have reused the same id after a late delivery.
func (p *Plugin) removePending(call *outboundCall) {
	p.mu.Lock()
	if p.pending[call.id] == call {
		delete(p.pending, call.id)
	}
	p.mu.Unlock()
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
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
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
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
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

// writeFrame serializes one Frame onto stdout. It uses writeMu, never mu: a
// blocked stdout write must not prevent Listen from completing pending calls.
func (p *Plugin) writeFrame(frame *protocol.Frame) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return protocol.WriteFrame(p.stdout, frame)
}

// Handle adapts a handler returning an arbitrary JSON-marshalable result to
// HandleFunc.
//
// Deprecated: new handlers should return json.RawMessage directly and be
// passed to Register without this wrapper.
func Handle(fn func(ctx context.Context, req *Request) (interface{}, error)) HandleFunc {
	return func(ctx context.Context, req *Request) (json.RawMessage, error) {
		res, err := fn(ctx, req)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return json.RawMessage("null"), nil
		}
		byt, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return byt, nil
	}
}

// Decode a Request to plugin internal struct
func Decode(src *Request, dst interface{}) error {
	if len(src.Payload) == 0 {
		return errors.New("payload is required")
	}
	return json.Unmarshal(src.Payload, dst)
}

// func ToPayload(src interface{}) json.RawMessage {
// 	b, err := json.Marshal(src)
// 	if err != nil {
// 	}
// 	return b
// }
