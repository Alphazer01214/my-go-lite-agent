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

// Request is one inbound Host req delivered to a HandleFunc.
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

// HandleFunc processes one Call payload and returns a result body (or error).
type HandleFunc func(req *Request) (json.RawMessage, error)

// Plugin is the in-process runtime: dispatches inbound req, completes outbound Call.
// Plugins never address by plugin name — only capability.method.
type Plugin struct {
	mu   sync.Mutex
	name string
	// handlers maps "capability.method" to a HandleFunc
	handlers map[string]HandleFunc
	// pending maps outbound Call id to its waiter
	pending map[string]chan *protocol.Frame
	// writeMu serializes stdout (Call + Dispatch may run concurrently)
	writeMu sync.Mutex
	stdin   *os.File
	stdout  *os.File

	seq atomic.Int32
}

// NewPlugin returns a Plugin bound to process stdio.
// name is identity only (logs / id prefix) — never used for routing.
func NewPlugin(name string) *Plugin {
	return &Plugin{
		name:     name,
		handlers: make(map[string]HandleFunc),
		pending:  make(map[string]chan *protocol.Frame),
		stdin:    os.Stdin,
		stdout:   os.Stdout,
	}
}

// Register binds capability.method to handler.
// Same-plugin re-register is last-wins (protocol §4.2).
func (p *Plugin) Register(capability string, method string, handler HandleFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[capability+"."+method] = handler
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
	p.mu.Lock()
	keys := make([]string, 0, len(p.handlers))
	for k := range p.handlers {
		keys = append(keys, k)
	}
	p.mu.Unlock()

	handlers := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		capability, method, _ := strings.Cut(k, ".")
		handlers = append(handlers, map[string]string{
			"capability": capability,
			"method":     method,
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
			go p.Dispatch(frame)
		case protocol.FrameResponse:
			p.complete(frame)
		default:
			// v1: evt / unknown types are ignored (forward compatible)
		}
	}
}

// fail tears down every in-flight outbound Call (stdin closed / read error).
// Blocked Call waiters receive a nil frame (closed channel) and return an error.
func (p *Plugin) fail() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ch := range p.pending {
		close(ch)
		delete(p.pending, id)
	}
}

// complete delivers a res to the waiting Call and drops the pending entry.
// Send happens under mu so it cannot race with fail()'s close.
func (p *Plugin) complete(frame *protocol.Frame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.pending[frame.ID]
	if !ok {
		return
	}
	delete(p.pending, frame.ID)
	ch <- frame
}

// Dispatch handles one inbound req and writes exactly one res (same id).
// Unknown capability.method → method_not_found; handler error → code or handler_error.
func (p *Plugin) Dispatch(frame *protocol.Frame) {
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
		out, err := h(&Request{
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

// Call invokes capability.method through Host (star topology).
// Blocks until res, fail(), or local write error. Never names a target plugin.
func (p *Plugin) Call(capability string, method string, payload json.RawMessage) (json.RawMessage, error) {
	id := fmt.Sprintf("%s-%d", p.name, p.seq.Add(1))
	ch := make(chan *protocol.Frame, 1)

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
		delete(p.pending, id)
		p.mu.Unlock()
		return nil, fmt.Errorf("write call %s.%s: %w", capability, method, err)
	}

	res := <-ch
	if res == nil {
		return nil, fmt.Errorf("call %s.%s: connection closed", capability, method)
	}
	if res.ErrorCode != "" || res.ErrorMsg != "" {
		code := res.ErrorCode
		if code == "" {
			code = CodeHandlerError
		}
		return nil, ErrCode(code, res.ErrorMsg)
	}
	return res.Payload, nil
}

func (p *Plugin) CallWithCallback(capability string, method string, payload json.RawMessage, callback func(event *Event)) (json.RawMessage, error) {

}

// Emit sends an evt Frame (no response expected), and no specific target
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
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return protocol.WriteFrame(p.stdout, frame)
}
