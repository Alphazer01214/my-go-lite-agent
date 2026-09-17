// Package protocol defines the Host↔Plugin Frame on stdio.
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Frame is one complete Host↔Plugin message: length-prefixed JSON body.
//
// Addressing (ADR-0030, Frame v5): Host routes by To (target plugin name) and
// forwards Payload opaquely. Cap/Method remain for the receiving Plugin's own
// dispatch — Host must not branch on domain capability names.
type Frame struct {
	V       int             `json:"v"`
	ID      string          `json:"id"`
	Type    string          `json:"type"` // req | res | evt
	To      string          `json:"to,omitempty"`
	Cap     string          `json:"cap,omitempty"`
	Method  string          `json:"method,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *FrameError     `json:"error,omitempty"`
}

// FrameError is the structured error carried on a res frame.
type FrameError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *FrameError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

const (
	TypeReq = "req"
	TypeRes = "res"
	TypeEvt = "evt"
)

// Version is the Frame protocol version carried in Frame.V.
// v2: presentation render kinds are markdown_text | message_text | summary_text.
// v5: Host addresses by To (plugin name) + opaque payload; Cap/Method are
// receiving-plugin dispatch only (ADR-0030).
// It versions the wire Frame only; the plugin.json "protocol" field is a
// separate manifest/UI-contract version (plugin.CurrentProtocol, ADR-0012).
const Version = 5

// maxFrameSize guards against corrupt length prefixes.
const maxFrameSize = 16 << 20

// WriteFrame encodes f as uint32 big-endian length + JSON body.
func WriteFrame(w io.Writer, f *Frame) error {
	body, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	if len(body) > maxFrameSize {
		return fmt.Errorf("frame too large: %d", len(body))
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write frame body: %w", err)
	}
	return nil
}

// ReadFrame decodes one length-prefixed JSON frame.
func ReadFrame(r io.Reader) (*Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > maxFrameSize {
		return nil, fmt.Errorf("invalid frame length: %d", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read frame body: %w", err)
	}
	var f Frame
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("unmarshal frame: %w", err)
	}
	return &f, nil
}
