// Package toolcontract defines the JSON envelope shared by Tool Providers and
// the tools plugin. It is an SDK helper for plugin authors, not a Host contract.
package toolcontract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"

	MaxArgumentsBytes = 1 << 20
	MaxContentBytes   = 1 << 20
	MaxMetaBytes      = 64 << 10
)

// Definition is the model-visible Tool registration plus its private Target.
type Definition struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputSchema      json.RawMessage `json:"input_schema"`
	Risk             string          `json:"risk"`
	SourcePlugin     string          `json:"source_plugin,omitempty"`
	TargetCapability string          `json:"target_capability"`
	TargetMethod     string          `json:"target_method"`
}

// CallInput is the tools.call payload.
type CallInput struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
}

// ProviderInput is delivered to a Tool Provider by the tools plugin.
type ProviderInput struct {
	Arguments json.RawMessage `json:"arguments"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
}

// Result is the only business result shape returned by a Tool Provider.
type Result struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// RegisterInput is the tools.register payload.
type RegisterInput struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputSchema      json.RawMessage `json:"input_schema"`
	TargetCapability string          `json:"target_capability"`
	TargetMethod     string          `json:"target_method"`
	Risk             string          `json:"risk"`
}

// RegisterOutput is the tools.register result.
type RegisterOutput struct {
	Registered bool `json:"registered"`
	Updated    bool `json:"updated"`
}

// Caller is the subset of pluginsdk.Plugin needed for registration.
type Caller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

// DecodeStrict decodes exactly one JSON object and rejects unknown fields.
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

// Payload decodes a ProviderInput and returns the arguments object.
func Payload(raw json.RawMessage) (ProviderInput, json.RawMessage, error) {
	var in ProviderInput
	if err := DecodeStrict(raw, &in); err != nil {
		return ProviderInput{}, nil, err
	}
	if len(in.Arguments) == 0 {
		in.Arguments = json.RawMessage(`{}`)
	}
	arguments, err := Object(in.Arguments, "arguments")
	if err != nil {
		return ProviderInput{}, nil, err
	}
	return in, arguments, nil
}

// Object validates that raw is a JSON object and returns it unchanged.
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

// Success builds a successful Tool Result.
func Success(content string, meta map[string]any) (json.RawMessage, error) {
	return Encode(Result{Content: content, Meta: meta})
}

// Failure builds a business failure Tool Result.
func Failure(code, message string, meta map[string]any) (json.RawMessage, error) {
	if meta == nil {
		meta = map[string]any{}
	}
	meta["code"] = code
	return Encode(Result{Content: message, IsError: true, Meta: meta})
}

// Encode validates size limits and marshals one Tool Result.
func Encode(result Result) (json.RawMessage, error) {
	if len(result.Content) > MaxContentBytes {
		result.Content = truncateUTF8(result.Content, MaxContentBytes)
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

// ParseResult validates the envelope returned by a Tool Provider.
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

func truncateUTF8(value string, limit int) string {
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

// RegisterAll retries provider registration until it succeeds or attempts run
// out. Registration failure never terminates the provider.
func RegisterAll(ctx context.Context, caller Caller, defs []Definition) error {
	if caller == nil {
		return errors.New("toolcontract: nil caller")
	}
	delay := 50 * time.Millisecond
	var last error
	for attempt := 0; attempt < 24; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if last = registerOnce(ctx, caller, defs); last == nil {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 2*time.Second {
			delay *= 2
		}
	}
	return fmt.Errorf("register tools after retries: %w", last)
}

func registerOnce(ctx context.Context, caller Caller, defs []Definition) error {
	for _, def := range defs {
		input := RegisterInput{
			Name:             def.Name,
			Description:      def.Description,
			InputSchema:      def.InputSchema,
			TargetCapability: def.TargetCapability,
			TargetMethod:     def.TargetMethod,
			Risk:             def.Risk,
		}
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if _, err := caller.Call(ctx, "tools", "register", payload); err != nil {
			return err
		}
	}
	return nil
}

// NormalizeRisk returns a canonical risk value or an empty string.
func NormalizeRisk(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case RiskLow:
		return RiskLow
	case RiskMedium:
		return RiskMedium
	case RiskHigh:
		return RiskHigh
	default:
		return ""
	}
}
