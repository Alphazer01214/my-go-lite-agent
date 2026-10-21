package main

// This file is the tools plugin's private wire format. It intentionally does
// not live in pluginsdk: tool registration and call envelopes are a business
// contract between this plugin and its providers, not a Host contract. Keep
// the JSON tags in sync with plugins/tools/communication.md.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	RiskLow      = 10
	RiskMedium   = 50
	RiskHigh     = 80
	RiskCritical = 90
	MaxRisk      = 100

	MaxArgumentsBytes = 1 << 20
	MaxContentBytes   = 1 << 20
	MaxMetaBytes      = 64 << 10
)

type Definition struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputSchema      json.RawMessage `json:"input_schema"`
	Risk             int             `json:"risk"`
	ParallelSafe     bool            `json:"parallel_safe,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
	SourcePlugin     string          `json:"source_plugin,omitempty"`
	TargetCapability string          `json:"target_capability"`
	TargetMethod     string          `json:"target_method"`
	SourceGeneration int             `json:"source_generation,omitempty"`
}

type CallInput struct {
	Name        string          `json:"name"`
	Arguments   json.RawMessage `json:"arguments"`
	SessionID   string          `json:"session_id,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Mode        string          `json:"mode,omitempty"`
}

type ProviderInput struct {
	Arguments   json.RawMessage `json:"arguments"`
	SessionID   string          `json:"session_id,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Mode        string          `json:"mode,omitempty"`
}

type Result struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type RegisterInput struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputSchema      json.RawMessage `json:"input_schema"`
	TargetCapability string          `json:"target_capability"`
	TargetMethod     string          `json:"target_method"`
	Risk             int             `json:"risk"`
	ParallelSafe     bool            `json:"parallel_safe,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
}

type RegisterOutput struct {
	Registered bool `json:"registered"`
	Updated    bool `json:"updated"`
}

type SyncInput struct {
	Tools []RegisterInput `json:"tools"`
}

type SyncOutput struct {
	Synced  bool `json:"synced"`
	Added   int  `json:"added"`
	Updated int  `json:"updated"`
	Removed int  `json:"removed"`
}

func DecodeStrict(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if err := ensureEOF(decoder); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("unexpected trailing JSON value")
	}
	return err
}

func Object(raw json.RawMessage, field string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, field+" must be a JSON object")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &value); err != nil || value == nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, field+" must be a JSON object")
	}
	if len(trimmed) > MaxArgumentsBytes {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, field+" exceeds 1 MiB")
	}
	return trimmed, nil
}

func Success(content string, meta map[string]any) (json.RawMessage, error) {
	return Encode(Result{Content: content, Meta: meta})
}

func Failure(code, message string, meta map[string]any) (json.RawMessage, error) {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["code"] = code
	return Encode(Result{Content: message, IsError: true, Meta: meta})
}

func Encode(result Result) (json.RawMessage, error) {
	if len(result.Content) > MaxContentBytes {
		result.Content = truncateString(result.Content, MaxContentBytes)
		result.Truncated = true
	}
	meta, err := json.Marshal(result.Meta)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "encode tool meta: "+err.Error())
	}
	if len(meta) > MaxMetaBytes {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "tool meta exceeds 64 KiB")
	}
	body, err := json.Marshal(result)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "encode tool result: "+err.Error())
	}
	return body, nil
}

func ParseResult(raw json.RawMessage) (Result, error) {
	var result Result
	if err := DecodeStrict(raw, &result); err != nil {
		return Result{}, err
	}
	if len(result.Content) > MaxContentBytes {
		return Result{}, pluginsdk.ErrCode("tool_result_invalid", "tool result content exceeds 1 MiB")
	}
	if result.Meta != nil {
		meta, err := json.Marshal(result.Meta)
		if err != nil || len(meta) > MaxMetaBytes {
			return Result{}, pluginsdk.ErrCode("tool_result_invalid", "tool result meta exceeds 64 KiB")
		}
	}
	return result, nil
}

func truncateString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		if value == "" {
			return ""
		}
		value = value[:len(value)-1]
	}
	return value
}
