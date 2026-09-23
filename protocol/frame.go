// Package protocol is the Host↔Plugin wire contract: length-prefixed JSON Frame.
// One definition shared by internal/host and pluginsdk — do not duplicate.
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Frame encoding: 4-byte big-endian length + UTF-8 JSON body.
const (
	// FrameVersion is the wire protocol version.
	FrameVersion = 1
	// FrameMaxSize is the max JSON body (16 MiB).
	FrameMaxSize = 16 << 20
)

// Frame.Type legal values.
const (
	FrameEvent    = "evt"
	FrameRequest  = "req"
	FrameResponse = "res"
)

// HostCapability is the built-in Host capability name (capability=host).
const HostCapability = "host"

// ErrFrameTooLarge is returned when a body exceeds FrameMaxSize.
var ErrFrameTooLarge = errors.New("frame_too_large")

// Frame is one complete Host↔Plugin message.
// Routing key is (capability, method); there is no target plugin name field.
// From is Host-injected on delivery — plugins leave it empty.
type Frame struct {
	Version    int             `json:"version"`
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	From       string          `json:"from,omitempty"`
	Capability string          `json:"capability,omitempty"`
	Method     string          `json:"method,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	// ErrorCode / ErrorMsg are always present; empty string on success.
	ErrorCode string `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

// WriteFrame writes one length-prefixed JSON Frame to w.
func WriteFrame(w io.Writer, f *Frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(data) > FrameMaxSize {
		return fmt.Errorf("frame size %d exceeds max %d: %w", len(data), FrameMaxSize, ErrFrameTooLarge)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadFrame reads one length-prefixed JSON Frame from r.
func ReadFrame(r io.Reader) (*Frame, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(header[:]))
	if size <= 0 || size > FrameMaxSize {
		return nil, fmt.Errorf("invalid frame length %d: %w", size, ErrFrameTooLarge)
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
