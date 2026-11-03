package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	maxCodeBytes = 512 * 1024
)

type providerRequest struct {
	Arguments json.RawMessage `json:"arguments,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	ContextID string          `json:"-"`
}

type toolResult struct {
	Content   string          `json:"content"`
	IsError   bool            `json:"is_error"`
	Truncated bool            `json:"truncated"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Method      string          `json:"method"`
}

type infoResponse struct {
	Capability string     `json:"capability"`
	Tools      []toolInfo `json:"tools"`
}

type executeArgs struct {
	Code           string            `json:"code,omitempty"`
	Path           string            `json:"path,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutMS      int               `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
}

type shellArguments struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Stdin          string            `json:"stdin,omitempty"`
	TimeoutMS      int               `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
}

type shellCall struct {
	Arguments shellArguments  `json:"arguments"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

func decodeProviderRequest(req *pluginsdk.Request) (*providerRequest, error) {
	if req == nil || len(req.Payload) == 0 {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "payload is required")
	}
	var input providerRequest
	if err := json.Unmarshal(req.Payload, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if len(input.Arguments) == 0 {
		input.Arguments = json.RawMessage(`{}`)
	}
	input.ContextID = req.ContextID
	return &input, nil
}

func decodeArgs(raw json.RawMessage, dst any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte(`{}`)
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return errors.New("arguments must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON")
		}
		return err
	}
	return nil
}

func checkContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "call deadline exceeded")
		}
		return pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "call cancelled")
	}
	return nil
}

func badArguments(format string, args ...any) error {
	return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, fmt.Sprintf(format, args...))
}
