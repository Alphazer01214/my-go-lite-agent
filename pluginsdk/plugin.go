package pluginsdk

import (
	"encoding/json"
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
	Capability string
	Method     string
	Payload    json.RawMessage
}

type Event struct {
	ID         string
	Capability string
	Method     string
	Payload    json.RawMessage
}

// HandleFunc processes one inbound req and returns a result body (or error).
type HandleFunc func(req *Request) (json.RawMessage, error)

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
	// pending maps outbound Call id to its waiter channel.
	// Close condition: res is delivered, or fail() on disconnect.
	pending map[string]chan *protocol.Frame
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
		pending:  make(map[string]chan *protocol.Frame),
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
	_, _ = p.Call(HostCapability, "register", payload)
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
		default:
			// v1: unknown types are ignored (forward compatible)
		}
	}
}

// fail tears down every in-flight outbound Call (stdin closed / read error).
// Waiters observe a closed channel and return connection closed.
func (p *Plugin) fail() {
	p.mu.Lock()
	chans := make([]chan *protocol.Frame, 0, len(p.pending))
	for id, ch := range p.pending {
		chans = append(chans, ch)
		delete(p.pending, id)
	}
	p.mu.Unlock()
	for _, ch := range chans {
		close(ch)
	}
}

// complete delivers a res to the waiting Call, then closes the channel (close = res received).
// Send/close happen outside mu so they cannot race with fail()'s close (map entry is claimed first).
func (p *Plugin) complete(frame *protocol.Frame) {
	p.mu.Lock()
	ch, ok := p.pending[frame.ID]
	if ok {
		delete(p.pending, frame.ID)
	}
	p.mu.Unlock()
	if !ok {
		return
	}
	ch <- frame
	close(ch)
}

// dispatch handles one inbound req and writes exactly one res (same id).
// Unknown capability.method → method_not_found; handler error → code or handler_error.
func (p *Plugin) dispatch(frame *protocol.Frame) {
	key := frame.Capability + "." + frame.Method
	p.mu.Lock()
	h, ok := p.handlers[key]
	p.mu.Unlock()

	res := &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         frame.ID,
		Type:       protocol.FrameResponse,
		Capability: frame.Capability,
		Method:     frame.Method,
	}
	if !ok {
		res.ErrorCode = CodeMethodNotFound
		res.ErrorMsg = fmt.Sprintf("no handler for %s", key)
	} else {
		out, err := h.handleFunc(&Request{
			ID:         frame.ID,
			Capability: frame.Capability,
			Method:     frame.Method,
			Payload:    frame.Payload,
		})
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
	_ = p.writeFrame(res)
}

// deliver routes an attributed evt to the matching pending Call waiter.
// Unattributed evt (empty id) is dropped — it must not pollute Call channels.
func (p *Plugin) deliver(frame *protocol.Frame) {
	if frame.ID == "" {
		return
	}
	p.mu.Lock()
	ch, ok := p.pending[frame.ID]
	p.mu.Unlock()
	if !ok {
		return
	}
	// Non-blocking: drop under backpressure so complete/fail cannot deadlock.
	select {
	case ch <- frame:
	default:
	}
}

func (p *Plugin) toEvent(frame *protocol.Frame) *Event {
	return &Event{
		ID:         frame.ID,
		Capability: frame.Capability,
		Method:     frame.Method,
		Payload:    frame.Payload,
	}
}

// waitResult drains the waiter until res (or channel close).
// Attributed evt frames go to callback when non-nil; otherwise they are discarded.
func (p *Plugin) waitResult(capability, method string, ch chan *protocol.Frame, callback func(*Event)) (json.RawMessage, error) {
	for {
		frame, ok := <-ch
		if !ok || frame == nil {
			return nil, fmt.Errorf("call %s.%s: connection closed", capability, method)
		}
		switch frame.Type {
		case protocol.FrameEvent:
			if callback != nil {
				callback(p.toEvent(frame))
			}
		case protocol.FrameResponse:
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

// callPending registers a waiter and writes the outbound req.
func (p *Plugin) callPending(capability, method string, payload json.RawMessage) (chan *protocol.Frame, error) {
	id := fmt.Sprintf("%s-%d", p.name, p.seq.Add(1))
	ch := make(chan *protocol.Frame, 32)

	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()

	// from is Host-injected on delivery; plugins leave it empty.
	frame := protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         id,
		Type:       protocol.FrameRequest,
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
			close(ch)
		}
		return nil, fmt.Errorf("write call %s.%s: %w", capability, method, err)
	}
	return ch, nil
}

// Call invokes capability.method through Host (star topology).
// Blocks until res, fail(), or local write error. Never names a target plugin.
// Attributed evt frames are discarded.
func (p *Plugin) Call(capability string, method string, payload json.RawMessage) (json.RawMessage, error) {
	ch, err := p.callPending(capability, method, payload)
	if err != nil {
		return nil, err
	}
	return p.waitResult(capability, method, ch, nil)
}

// CallWithCallback is Call plus a stream of attributed evt frames delivered to callback
// until the final res. callback must be fast and non-blocking.
func (p *Plugin) CallWithCallback(capability string, method string, payload json.RawMessage, callback func(event *Event)) (json.RawMessage, error) {
	ch, err := p.callPending(capability, method, payload)
	if err != nil {
		return nil, err
	}
	return p.waitResult(capability, method, ch, callback)
}

// Emit sends an unattributed evt Frame (no response expected).
func (p *Plugin) Emit(capability string, method string, payload json.RawMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return protocol.WriteFrame(p.stdout, &protocol.Frame{
		Version:    protocol.FrameVersion,
		Type:       protocol.FrameEvent,
		Capability: capability,
		Method:     method,
		Payload:    payload,
	})
}

// EmitWithID sends an attributed evt (id points at an in-flight Call).
func (p *Plugin) EmitWithID(id string, capability string, method string, payload json.RawMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return protocol.WriteFrame(p.stdout, &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         id,
		Type:       protocol.FrameEvent,
		Capability: capability,
		Method:     method,
		Payload:    payload,
	})
}

// writeFrame serializes one Frame onto stdout.
func (p *Plugin) writeFrame(frame *protocol.Frame) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return protocol.WriteFrame(p.stdout, frame)
}
