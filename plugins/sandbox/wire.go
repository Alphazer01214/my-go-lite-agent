package main

// This file is the sandbox plugin's private wire format. It intentionally does
// not live in pluginsdk: the adjudication payload is a business contract
// between this plugin and the seams it consumes, not a Host contract. Keep the
// JSON tags in sync with plugins/sandbox/communication.md.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// Plugin-level error codes. The sandbox_ prefix is reserved for this plugin
// in docs/design/plugins/00-conventions.md §6.
const (
	CodeDenied       = "sandbox_denied"
	CodeRuleInvalid  = "sandbox_rule_invalid"
	CodeRuleNotFound = "sandbox_rule_not_found"
)

// MaxArgumentsBytes bounds one argument object. The tools plugin uses the
// same limit for the arguments it forwards.
const MaxArgumentsBytes = 1 << 20

// toolResult is the Tool Result envelope owned by the tools plugin
// (plugins/tools/communication.md §6). The sandbox returns it on the
// short-circuit path so the model sees the reason instead of a transport
// error.
type toolResult struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// seamPayload is the union of the two seam payload shapes the sandbox
// consumes: tools.call carries the tool name, the provider seams carry the
// arguments only. The intent fields are flat on both seams, matching the
// current tools contract. Unknown fields are ignored so a consumer keeps
// working when the caller adds one.
type seamPayload struct {
	Name        string          `json:"name,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Mode        string          `json:"mode,omitempty"`
}

// DecodeStrict decodes a payload this plugin owns. Unknown fields, wrong types
// and trailing values are all bad_arguments.
func DecodeStrict(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "unexpected trailing JSON value")
		}
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	return nil
}

// DecodeSeam decodes a payload owned by another plugin. Unknown fields are
// ignored: a seam payload may grow without breaking this consumer.
func DecodeSeam(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "payload is required")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	return nil
}

// Arguments decodes the argument object of a seam payload. A missing or empty
// arguments field is an empty object.
func Arguments(raw json.RawMessage) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	if len(trimmed) > MaxArgumentsBytes {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "arguments exceeds 1 MiB")
	}
	if trimmed[0] != '{' {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "arguments must be a JSON object")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &value); err != nil || value == nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "arguments must be a JSON object")
	}
	return value, nil
}

// ArgumentString reads one string argument. A missing or non-string value is
// an empty string: a rule that needs the field simply does not match.
func ArgumentString(args map[string]json.RawMessage, key string) string {
	raw, ok := args[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// ArgumentStrings reads one string array argument.
func ArgumentStrings(args map[string]json.RawMessage, key string) []string {
	raw, ok := args[key]
	if !ok {
		return nil
	}
	var value []string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

// Digest is the argument digest written to the audit entry. It never carries
// the argument text itself.
func Digest(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

// withArguments replaces the arguments field of a seam payload. Every other
// field is preserved byte for byte, including fields this plugin does not
// know about.
func withArguments(payload json.RawMessage, arguments json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || fields == nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "payload must be a JSON object")
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	fields["arguments"] = arguments
	body, err := json.Marshal(fields)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	return body, nil
}

// applyTemplate merges a modify template over the original arguments. The
// template is an overlay: keys it names are replaced, keys it does not name
// are kept. The result is the complete argument set the provider receives.
func applyTemplate(original json.RawMessage, template json.RawMessage) (json.RawMessage, error) {
	base, err := Arguments(original)
	if err != nil {
		return nil, err
	}
	overlay, err := Arguments(template)
	if err != nil {
		return nil, pluginsdk.ErrCode(CodeRuleInvalid, "modified_arguments must be a JSON object")
	}
	for key, value := range overlay {
		base[key] = value
	}
	body, err := json.Marshal(base)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	return body, nil
}

// encodeResult renders a Tool Result envelope.
func encodeResult(result toolResult) (json.RawMessage, error) {
	body, err := json.Marshal(result)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	return body, nil
}

// mustJSON renders a value for an outbound call.
func mustJSON(v any) json.RawMessage {
	body, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return body
}

// describeScope renders a rule scope for a human readable reason.
func describeScope(scope ruleScope) string {
	parts := make([]string, 0, 5)
	if scope.Tool != "" {
		parts = append(parts, "tool="+scope.Tool)
	}
	if scope.Capability != "" {
		parts = append(parts, "capability="+scope.Capability)
	}
	if scope.Method != "" {
		parts = append(parts, "method="+scope.Method)
	}
	if scope.PathGlob != "" {
		parts = append(parts, "path="+scope.PathGlob)
	}
	if scope.CommandPattern != "" {
		parts = append(parts, "command="+scope.CommandPattern)
	}
	if len(parts) == 0 {
		return "any call"
	}
	return fmt.Sprint(parts)
}
