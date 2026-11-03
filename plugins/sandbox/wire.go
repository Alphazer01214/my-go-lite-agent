package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const maxArgumentsBytes = 1 << 20

type toolResult struct {
	Content   string         `json:"content"`
	IsError   bool           `json:"is_error"`
	Truncated bool           `json:"truncated"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// seamPayload is the union of the two seam shapes: tool.call carries a name,
// provider seams carry only arguments. Intent fields are flat on both.
type seamPayload struct {
	Name        string          `json:"name,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	SessionID   string          `json:"session_id,omitempty"`
	Workspace   string          `json:"workspace,omitempty"`
	WorkspaceID string          `json:"workspace_id,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Meta        json.RawMessage `json:"meta,omitempty"`
}

// DecodeSeam decodes a payload owned by another plugin. Unknown fields are
// ignored so a seam may grow without breaking this consumer.
func DecodeSeam(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "payload is required")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	return nil
}

// DecodeStrict decodes a payload this plugin owns.
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

func Arguments(raw json.RawMessage) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	if len(trimmed) > maxArgumentsBytes {
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

func Digest(raw json.RawMessage) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

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

// applyTemplate overlays the modify template on the original arguments.
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

func encodeResult(result toolResult) (json.RawMessage, error) {
	body, err := json.Marshal(result)
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	return body, nil
}

func commandLine(args map[string]json.RawMessage) string {
	command := ArgumentString(args, "command")
	extra := ArgumentStrings(args, "args")
	if len(extra) == 0 {
		return command
	}
	return strings.Join(append([]string{command}, extra...), " ")
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sandbox: "+format+"\n", args...)
}
