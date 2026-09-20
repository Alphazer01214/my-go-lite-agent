package host

import "encoding/json"

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

type FrameError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
