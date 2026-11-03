package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type providerRequest struct {
	Arguments json.RawMessage `json:"arguments,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	ContextID string          `json:"-"`
}

type providerTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Method      string          `json:"method"`
}

type providerInfo struct {
	Capability string         `json:"capability"`
	Tools      []providerTool `json:"tools"`
}

type providerToolResult struct {
	Content   string          `json:"content"`
	IsError   bool            `json:"is_error"`
	Truncated bool            `json:"truncated"`
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

func decodeProviderArgs(raw json.RawMessage, dst any) error {
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

func providerToolSuccess(content string, meta any) providerToolResult {
	var raw json.RawMessage
	if meta != nil {
		raw, _ = json.Marshal(meta)
	}
	return providerToolResult{Content: content, Meta: raw}
}

func providerToolFailure(code, message string) providerToolResult {
	meta, _ := json.Marshal(map[string]string{"code": code})
	return providerToolResult{Content: message, IsError: true, Meta: meta}
}

func providerError(scope string, err error) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, fmt.Sprintf("%s: %v", scope, err))
}
