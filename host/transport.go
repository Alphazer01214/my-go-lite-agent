package host

import (
	"encoding/binary"
	"encoding/json"
	"errors"
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
	// Err is the structured error carried on a res frame.
	Err *FrameError `json:"err,omitempty"`
}

// FrameError is the structured error on a res frame. It implements error.
type FrameError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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

// Is reports whether target is a *FrameError with the same Code.
func (e *FrameError) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	t, ok := target.(*FrameError)
	return ok && t != nil && t.Code == e.Code
}

// Errorf builds a *FrameError with wire code and formatted message.
func Errorf(code, format string, args ...any) *FrameError {
	return &FrameError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// AsFrameError unwraps err to *FrameError when possible.
func AsFrameError(err error) (*FrameError, bool) {
	if err == nil {
		return nil, false
	}
	var fe *FrameError
	if errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}

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

func (h *Host) routeRequest(from string, frame *Frame) error {
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

func (h *Host) forward(from string, to string, frame *Frame) error {
	if err := h.alive(to); err != nil {
		return err
	}

	h.mu.Lock()
	h.seq++
	forwardID := ForwardIDPrefix + fmt.Sprintf("%d", h.seq)
	wt := &wait{
		kind:       WaitPlugin,
		caller:     from,
		target:     to,
		frameID:    frame.ID,
		capability: frame.Capability,
		method:     frame.Method,
		payload:    append(json.RawMessage(nil), frame.Payload...),
	}
	h.pending[forwardID] = wt
	p := h.plugins[to]
	h.mu.Unlock()

	if p == nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return Errorf(CodePluginNotMounted, "plugin %s not mounted", to)
	}

	out := *frame
	out.ID = forwardID
	out.To = to
	if err := h.write(out); err != nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return err
	}
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
