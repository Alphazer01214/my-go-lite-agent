package webui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tomori/my-go-lite-agent/protocol"
)

const BrowserVersion = 1

type browserCall struct {
	Version    int             `json:"version"`
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Capability string          `json:"capability"`
	Method     string          `json:"method"`
	Payload    json.RawMessage `json:"payload"`
}

type rawBrowserCall struct {
	Version    int             `json:"version"`
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	From       json.RawMessage `json:"from,omitempty"`
	Capability string          `json:"capability"`
	Method     string          `json:"method"`
	Payload    json.RawMessage `json:"payload"`
}

func decodeBrowserCall(reader io.Reader) (browserCall, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()

	var raw rawBrowserCall
	if err := decoder.Decode(&raw); err != nil {
		return browserCall{}, fmt.Errorf("decode browser call: %w", err)
	}
	if raw.From != nil {
		return browserCall{}, errors.New("browser call from must not be provided")
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return browserCall{}, errors.New("browser call must contain one JSON object")
		}
		return browserCall{}, fmt.Errorf("decode browser call trailing data: %w", err)
	}

	if raw.Version != BrowserVersion {
		return browserCall{}, fmt.Errorf("browser call version must be %d", BrowserVersion)
	}
	if raw.ID == "" {
		return browserCall{}, errors.New("browser call id is required")
	}
	if raw.Type != protocol.FrameRequest {
		return browserCall{}, fmt.Errorf("browser call type must be %q", protocol.FrameRequest)
	}
	if raw.Capability == "" {
		return browserCall{}, errors.New("browser call capability is required")
	}
	if raw.Method == "" {
		return browserCall{}, errors.New("browser call method is required")
	}

	payload := append(json.RawMessage(nil), raw.Payload...)
	if len(bytes.TrimSpace(payload)) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		payload = json.RawMessage(`{}`)
	} else {
		var compact bytes.Buffer
		if err := json.Compact(&compact, payload); err != nil {
			return browserCall{}, fmt.Errorf("compact browser call payload: %w", err)
		}
		payload = compact.Bytes()
	}

	return browserCall{
		Version:    raw.Version,
		ID:         raw.ID,
		Type:       raw.Type,
		Capability: raw.Capability,
		Method:     raw.Method,
		Payload:    payload,
	}, nil
}

func browserFrame(source *protocol.Frame, requestID string) (*protocol.Frame, error) {
	if source == nil {
		return nil, errors.New("browser frame source is required")
	}
	if requestID == "" {
		return nil, errors.New("browser request id is required")
	}
	if source.Type != protocol.FrameEvent && source.Type != protocol.FrameResponse {
		return nil, fmt.Errorf("browser frame type must be %q or %q", protocol.FrameEvent, protocol.FrameResponse)
	}

	out := *source
	out.Version = BrowserVersion
	out.ID = requestID
	out.Payload = append(json.RawMessage(nil), source.Payload...)
	return &out, nil
}
