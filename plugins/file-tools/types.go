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
	defaultReadBytes  = 256 * 1024
	maxReadBytes      = 1 << 20
	maxWriteBytes     = 1 << 20
	maxEditFileBytes  = 4 << 20
	defaultGlobLimit  = 200
	maxGlobLimit      = 1000
	defaultGrepMatch  = 200
	maxGrepMatch      = 1000
	defaultGrepFiles  = 2000
	maxGrepFiles      = 10000
	maxGrepFileBytes  = 1 << 20
	maxGrepOutputSize = 900 * 1024
)

type providerRequest struct {
	Arguments json.RawMessage `json:"arguments,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
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

func success(content string, meta any) toolResult {
	var raw json.RawMessage
	if meta != nil {
		raw, _ = json.Marshal(meta)
	}
	return toolResult{Content: content, Meta: raw}
}

func failure(code, message string) toolResult {
	meta, _ := json.Marshal(map[string]string{"code": code})
	return toolResult{Content: message, IsError: true, Meta: meta}
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
