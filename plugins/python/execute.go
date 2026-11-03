package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) execute(ctx context.Context, req *providerRequest, in *executeArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}

	code := in.Code
	path := strings.TrimSpace(in.Path)
	if (code == "") == (path == "") {
		return toolResult{}, badArguments("exactly one of code or path is required")
	}
	for index, arg := range in.Args {
		if strings.ContainsRune(arg, 0) {
			return toolResult{}, badArguments("args[%d] contains NUL", index)
		}
	}

	mode := "code"
	shellArgs := shellArguments{
		Command:        p.interpreter,
		Cwd:            in.Cwd,
		Env:            in.Env,
		TimeoutMS:      in.TimeoutMS,
		MaxOutputBytes: in.MaxOutputBytes,
	}
	if code != "" {
		if !utf8.ValidString(code) || strings.ContainsRune(code, 0) {
			return toolResult{}, badArguments("code must be valid UTF-8 without NUL")
		}
		if len(code) > maxCodeBytes {
			return toolResult{}, badArguments("code exceeds %d bytes; use path mode for larger programs", maxCodeBytes)
		}
		shellArgs.Args = append([]string{"-"}, in.Args...)
		shellArgs.Stdin = code
	} else {
		mode = "path"
		script, err := resolveScript(req.Workspace, path)
		if err != nil {
			return toolResult{}, err
		}
		shellArgs.Args = append([]string{script}, in.Args...)
	}

	payload, err := json.Marshal(shellCall{
		Arguments: shellArgs,
		SessionID: req.SessionID,
		Workspace: req.Workspace,
		CallID:    req.CallID,
		Meta:      req.Meta,
	})
	if err != nil {
		return toolResult{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "encode shell call: "+err.Error())
	}

	var raw json.RawMessage
	if req.ContextID != "" {
		raw, err = p.caller.CallInContext(ctx, req.ContextID, "shell", "execute", payload)
	} else {
		raw, err = p.caller.Call(ctx, "shell", "execute", payload)
	}
	if err != nil {
		return toolResult{}, mapShellError(err)
	}

	var result toolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return toolResult{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "invalid shell result: "+err.Error())
	}

	extra := map[string]any{
		"mode":        mode,
		"interpreter": p.interpreter,
	}
	if mode == "path" {
		extra["path"] = path
	}
	result.Meta = mergeMeta(result.Meta, extra)
	return result, nil
}

func resolveScript(workspace, raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", errPathInvalid("path contains NUL")
	}

	var path string
	if filepath.IsAbs(raw) {
		path = filepath.Clean(raw)
	} else {
		base := workspace
		if base == "" {
			base = "."
		}
		path = filepath.Join(base, raw)
		if !filepath.IsAbs(path) {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return "", errPathInvalid(err.Error())
			}
			path = absolute
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", pluginsdk.ErrCode(codePathNotFound, "file not found: "+raw)
		}
		return "", errPathInvalid(err.Error())
	}
	if info.IsDir() {
		return "", pluginsdk.ErrCode(codePathIsDirectory, "path is a directory: "+raw)
	}
	if !info.Mode().IsRegular() {
		return "", errPathInvalid("path is not a regular file: " + raw)
	}
	return path, nil
}

func mergeMeta(raw json.RawMessage, extra map[string]any) json.RawMessage {
	values := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(raw)) > 0 {
		_ = json.Unmarshal(raw, &values)
		if values == nil {
			values = map[string]json.RawMessage{}
		}
	}
	for key, value := range extra {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		values[key] = encoded
	}
	out, err := json.Marshal(values)
	if err != nil {
		return raw
	}
	return out
}

func mapShellError(err error) error {
	switch pluginsdk.Code(err) {
	case "command_not_found":
		return pluginsdk.ErrCode(codeInterpreterNotFound, err.Error())
	case "method_not_found", "plugin_not_mounted", "plugin_down", "plugin_disabled", "plugin_not_ready":
		return pluginsdk.ErrCode(codeShellUnavailable, err.Error())
	default:
		return err
	}
}
