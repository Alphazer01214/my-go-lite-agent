package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Frame 线类型与编解码的唯一定义在 protocol；Host 经别名使用（payloads.go）。

type CallResult struct {
	Frame  *Frame
	Events []*Frame
}

func (h *Host) route(from string, frame *Frame) {
	switch frame.Type {
	case FrameRequest:
		h.handleRequest(from, frame)
	case FrameResponse:
		h.handleResponse(frame)
	case FrameEvent:
		h.handleEvent(from, frame)
	default:
		return
	}
}

// handleRequest only accept req with frame.ID = "plugin-xxx"
// Target is resolved from the (capability, method) route table; self-call is a legal loopback.
func (h *Host) handleRequest(from string, frame *Frame) error {
	if frame.Capability == HostCapability {
		// TODO: host.* method dispatch
		err := fmt.Errorf("no handler for host.%s: %w", frame.Method, ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	if frame.Capability == "" || frame.Method == "" {
		err := fmt.Errorf("capability and method are required: %w", ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	owner := h.routeOwner(frame.Capability)
	if owner == "" {
		err := fmt.Errorf("no owner for %s.%s: %w", frame.Capability, frame.Method, ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	if err := h.forward(from, owner, frame); err != nil {
		code := CodeRouteFailed
		// map sentinel errors to stable wire codes
		switch {
		case errors.Is(err, ErrPluginNotMounted):
			code = CodePluginNotMounted
		case errors.Is(err, ErrPluginDown):
			code = CodePluginDown
		case errors.Is(err, ErrPluginDisabled):
			code = CodePluginDisabled
		}
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	return nil
}

// routeOwner resolves the owning plugin process name for a capability.
// Internal only — never exposed as a Frame addressing field.
func (h *Host) routeOwner(capability string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.provides[capability]
}

// handleResponse only accept res with frame.ID = "fwd-xxx"
func (h *Host) handleResponse(frame *Frame) error {
	h.mu.Lock()
	wt, ok := h.pending[frame.ID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("response for unknown frame: %s", frame.ID)
	}
	delete(h.pending, frame.ID)
	h.mu.Unlock()
	// TODO host

	// if is plugin.
	res := *frame
	res.ID = wt.frameID
	if err := h.write(wt.caller, &res); err != nil {
		return err
	}

	return nil
}

// handleEvent only accept EVENT with frame.ID = "plugin-xxx"
func (h *Host) handleEvent(from string, frame *Frame) error {
	// TODO: broadcast to all plugins that have declared the capability
	h.mu.Lock()
	wt, ok := h.pending[frame.ID]
	if !ok {
		h.mu.Unlock()
		return nil
	}
	// TODO: handle the stream (callback) event
	wt.events = append(wt.events, frame)
	h.mu.Unlock()
	return nil
}

// replyError synthesizes a res to the **caller** by write
func (h *Host) replyError(caller string, originalID string, capability string,
	method string, code string, message string) {
	_ = h.write(caller, &Frame{
		ID:         originalID,
		Type:       FrameResponse,
		Capability: capability,
		Method:     method,
		ErrorCode:  code,
		ErrorMsg:   message,
	})
}

// forward sends a req frame from one plugin to another, and waits for the response.
// owner is the resolved plugin process name (Host-internal; not a Frame field).
// Self-call (owner == from) is a legal loopback.
func (h *Host) forward(from string, owner string, frame *Frame) error {
	if err := h.alive(owner); err != nil {
		return err
	}

	h.mu.Lock()
	// Here handles the frame original plugin(from) sent
	// the frame is turned into a wait in pending, with the **fwd** key
	// the fwd key (fwd-xxxxxx) marks the frame is waiting for a response
	h.seq++
	wt := &wait{
		kind:       WaitPlugin,
		caller:     from,
		target:     owner,
		frameID:    frame.ID,
		capability: frame.Capability,
		method:     frame.Method,
		payload:    append(json.RawMessage(nil), frame.Payload...),
	}
	// pending[fwd-xxx] = wait{plugin-xxx}
	forwardID := fmt.Sprintf("fwd-%d", h.seq)
	h.pending[forwardID] = wt
	p := h.plugins[owner]
	h.mu.Unlock()

	if p == nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return fmt.Errorf("plugin %s not mounted: %w", owner, ErrPluginNotMounted)
	}

	// Here the frame is forwarded, with the fwd-xxx
	forwardFrame := *frame
	forwardFrame.ID = forwardID

	if err := h.write(owner, &forwardFrame); err != nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return err
	}
	return nil

}

func (h *Host) broadcast(from string, frame *Frame) error {
	return nil
}

func (h *Host) write(to string, frame *Frame) error {

	if err := h.alive(to); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	plg := h.plugins[to]
	plg.mu.Lock()
	defer plg.mu.Unlock()
	if err := WriteFrame(plg.stdin, frame); err != nil {
		return err
	}
	return nil
}

func (h *Host) read(name string, gen int, stdout io.Reader) {
	for {
		frame, err := ReadFrame(stdout)
		if err != nil {
			h.hlog(fmt.Sprintf("read frame error: %v", err))
			h.markUnhealthy(name, gen)
			return
		}
		h.route(name, frame)
	}
}
