package host

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Frame is one complete Host↔Plugin message: length-prefixed JSON body.
type Frame struct {
	// Version is the protocol version.
	Version int `json:"version"`
	// ID is the unique frame ID.
	ID string `json:"id"`
	// Type is the frame type evt/req/res.
	Type string `json:"type"`
	// To is the target plugin name. Empty for evt.
	To string `json:"to,omitempty"`
	// Capability is the capability name. Empty for evt.
	Capability string `json:"capability,omitempty"`
	// Method is the method name. Empty for evt.
	Method string `json:"method,omitempty"`
	// Payload is the payload of the frame.
	Payload json.RawMessage `json:"payload,omitempty"`
	// ErrorMsg is the error message.
	ErrorMsg string `json:"error_msg,omitempty"`
}

type CallResult struct {
	Frame  *Frame
	Events []*Frame
}

func (e *FrameError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// WriteFrame writes a Frame to the given writer, prefixing it with its length.
func WriteFrame(w io.Writer, f *Frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(data) > FrameMaxSize {
		return Errorf(CodeFrameTooLarge, "frame size %d exceeds max %d", len(data), FrameMaxSize)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}

	return nil
}

// ReadFrame reads a Frame from the given reader, expecting a length-prefixed JSON body.
func ReadFrame(r io.Reader) (*Frame, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size <= 0 || size > FrameMaxSize {
		return nil, Errorf(CodeFrameTooLarge, "invalid frame length %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	var frame Frame
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, err
	}
	return &frame, nil
}

func (h *Host) route(from string, frame *Frame) error {
	switch frame.Type {
	case FrameRequest:
		return h.handleRequest(from, frame)
	case FrameResponse:
		return h.handleResponse(frame)
	case FrameEvent:
		return h.handleEvent(from, frame)
	default:
		return fmt.Errorf("unknown frame type")
	}
}

// handleRequest only accept req with frame.ID = "plugin-xxx"
func (h *Host) handleRequest(from string, frame *Frame) error {
	if frame.To == HostCapability || frame.Capability == HostCapability {
		// TODO: host.* method dispatch
		return Errorf(CodeMethodNotFound, "no handler for host.%s", frame.Method)
	}
	if frame.To == "" {
		return ErrToRequired
	}
	if frame.To == from {
		return Errorf(CodeCallSelf, "plugin %s cannot call itself", from)
	}
	return h.forward(from, frame.To, frame)
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

// replyError sends the error to the **caller** by write
func (h *Host) replyError(caller string, originalID string, capability string,
	method string, message string) {
	_ = h.write(caller, &Frame{
		ID:         originalID,
		Capability: capability,
		Method:     method,
		ErrorMsg:   message,
	})
}

// forward sends a req frame from one plugin to another, and waits for the response.
// Only called when the endpoint is a plugin (not itself, not host)
func (h *Host) forward(from string, to string, frame *Frame) error {
	if err := h.alive(to); err != nil {
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
		target:     to,
		frameID:    frame.ID,
		capability: frame.Capability,
		method:     frame.Method,
		payload:    append(json.RawMessage(nil), frame.Payload...),
	}
	// pending[fwd-xxx] = wait{plugin-xxx}
	forwardID := fmt.Sprintf("fwd-%d", h.seq)
	h.pending[forwardID] = wt
	p := h.plugins[to]
	h.mu.Unlock()

	if p == nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return Errorf(CodePluginNotMounted, "plugin %s not mounted", to)
	}

	// Here the frame is forwarded, with the fwd-xxx
	forwardFrame := *frame
	forwardFrame.ID = forwardID
	forwardFrame.To = frame.To

	if err := h.write(to, &forwardFrame); err != nil {
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
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.alive(to); err != nil {
		return err
	}
	plg := h.plugins[to]
	plg.mu.Lock()
	defer plg.mu.Unlock()
	if err := WriteFrame(plg.stdin, frame); err != nil {
		return err
	}
	return nil
}
