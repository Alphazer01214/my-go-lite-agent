package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type fakeCaller struct {
	capability string
	method     string
	contextID  string
	payload    json.RawMessage
	response   json.RawMessage
	err        error
}

func (f *fakeCaller) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.capability = capability
	f.method = method
	f.payload = payload
	return f.response, f.err
}

func (f *fakeCaller) CallInContext(_ context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.contextID = contextID
	return f.Call(context.Background(), capability, method, payload)
}

func shellResult(content string, meta map[string]any, isError bool) json.RawMessage {
	var rawMeta json.RawMessage
	if meta != nil {
		rawMeta, _ = json.Marshal(meta)
	}
	raw, _ := json.Marshal(toolResult{Content: content, IsError: isError, Meta: rawMeta})
	return raw
}

func testPlugin(c caller) *plugin {
	return &plugin{caller: c, interpreter: "python3", interpreterFound: true}
}

func decodeShellCall(t *testing.T, raw json.RawMessage) shellCall {
	t.Helper()
	var call shellCall
	if err := json.Unmarshal(raw, &call); err != nil {
		t.Fatalf("shell call: %v (%s)", err, raw)
	}
	return call
}

func decodeResultMeta(t *testing.T, result toolResult) map[string]any {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal(result.Meta, &meta); err != nil {
		t.Fatalf("meta: %v (%s)", err, result.Meta)
	}
	return meta
}

func TestExecuteCodeMode(t *testing.T) {
	fake := &fakeCaller{response: shellResult("ok", map[string]any{"exit_code": 0}, false)}
	p := testPlugin(fake)

	result, err := p.execute(context.Background(), &providerRequest{
		Workspace: "/tmp",
		SessionID: "s-1",
		CallID:    "c-1",
	}, &executeArgs{
		Code: "print(1)",
		Args: []string{"a"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fake.capability != "shell" || fake.method != "execute" {
		t.Fatalf("call = %s.%s", fake.capability, fake.method)
	}
	call := decodeShellCall(t, fake.payload)
	if call.Arguments.Command != "python3" {
		t.Fatalf("command = %q", call.Arguments.Command)
	}
	if call.Arguments.Stdin != "print(1)" {
		t.Fatalf("stdin = %q", call.Arguments.Stdin)
	}
	if len(call.Arguments.Args) != 2 || call.Arguments.Args[0] != "-" || call.Arguments.Args[1] != "a" {
		t.Fatalf("args = %v", call.Arguments.Args)
	}
	if call.SessionID != "s-1" || call.CallID != "c-1" {
		t.Fatalf("call metadata = %+v", call)
	}
	if result.Content != "ok" {
		t.Fatalf("content = %q", result.Content)
	}
	meta := decodeResultMeta(t, result)
	if meta["mode"] != "code" || meta["interpreter"] != "python3" {
		t.Fatalf("meta = %s", result.Meta)
	}
}

func TestExecutePathMode(t *testing.T) {
	workspace := t.TempDir()
	script := filepath.Join(workspace, "script.py")
	if err := os.WriteFile(script, []byte("print(1)"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	fake := &fakeCaller{response: shellResult("ok", nil, false)}
	p := testPlugin(fake)

	result, err := p.execute(context.Background(), &providerRequest{Workspace: workspace}, &executeArgs{
		Path: "script.py",
		Args: []string{"x"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	call := decodeShellCall(t, fake.payload)
	if len(call.Arguments.Args) != 2 || call.Arguments.Args[0] != script || call.Arguments.Args[1] != "x" {
		t.Fatalf("args = %v", call.Arguments.Args)
	}
	if call.Arguments.Stdin != "" {
		t.Fatalf("stdin = %q", call.Arguments.Stdin)
	}
	meta := decodeResultMeta(t, result)
	if meta["mode"] != "path" || meta["path"] != "script.py" {
		t.Fatalf("meta = %s", result.Meta)
	}
}

func TestExecuteAbsolutePathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	script := filepath.Join(outside, "script.py")
	if err := os.WriteFile(script, []byte("print(1)"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	fake := &fakeCaller{response: shellResult("ok", nil, false)}
	p := testPlugin(fake)

	if _, err := p.execute(context.Background(), &providerRequest{Workspace: workspace}, &executeArgs{
		Path: script,
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	call := decodeShellCall(t, fake.payload)
	if call.Arguments.Args[0] != script {
		t.Fatalf("args = %v", call.Arguments.Args)
	}
}

func TestExecuteRequiresExactlyOneOfCodeAndPath(t *testing.T) {
	p := testPlugin(&fakeCaller{})
	ctx := context.Background()

	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("neither err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Code: "print(1)", Path: "x.py"}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("both err = %v", err)
	}
}

func TestExecutePathErrors(t *testing.T) {
	p := testPlugin(&fakeCaller{})
	ctx := context.Background()
	workspace := t.TempDir()

	if _, err := p.execute(ctx, &providerRequest{Workspace: workspace}, &executeArgs{Path: "missing.py"}); pluginsdk.Code(err) != codePathNotFound {
		t.Fatalf("missing err = %v", err)
	}
	dir := filepath.Join(workspace, "dir.py")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{Workspace: workspace}, &executeArgs{Path: dir}); pluginsdk.Code(err) != codePathIsDirectory {
		t.Fatalf("directory err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{Workspace: workspace}, &executeArgs{Path: "bad\x00path.py"}); pluginsdk.Code(err) != codePathInvalid {
		t.Fatalf("NUL path err = %v", err)
	}
}

func TestExecuteCodeValidation(t *testing.T) {
	p := testPlugin(&fakeCaller{})
	ctx := context.Background()

	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Code: "bad\x00code"}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("NUL code err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Code: strings.Repeat("a", maxCodeBytes+1)}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("large code err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Code: "print(1)", Args: []string{"bad\x00arg"}}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("NUL arg err = %v", err)
	}
}

func TestExecuteShellErrorMapping(t *testing.T) {
	ctx := context.Background()

	for _, test := range []struct {
		code string
		want string
	}{
		{code: "command_not_found", want: codeInterpreterNotFound},
		{code: "method_not_found", want: codeShellUnavailable},
		{code: "plugin_not_mounted", want: codeShellUnavailable},
		{code: "process_start_failed", want: "process_start_failed"},
	} {
		p := testPlugin(&fakeCaller{err: pluginsdk.ErrCode(test.code, "boom")})
		_, err := p.execute(ctx, &providerRequest{}, &executeArgs{Code: "print(1)"})
		if pluginsdk.Code(err) != test.want {
			t.Fatalf("%s err = %v, want %s", test.code, err, test.want)
		}
	}
}

func TestExecuteUsesContextID(t *testing.T) {
	fake := &fakeCaller{response: shellResult("ok", nil, false)}
	p := testPlugin(fake)
	if _, err := p.execute(context.Background(), &providerRequest{ContextID: "ctx-1"}, &executeArgs{Code: "print(1)"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fake.contextID != "ctx-1" {
		t.Fatalf("contextID = %q", fake.contextID)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}

func interpreterFile(dir string) string {
	name := "python"
	if runtime.GOOS == "windows" {
		name = "python.exe"
	}
	return filepath.Join(dir, name)
}

func TestLookPathCommandSkipsRelativePathEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("PATHEXT", ".EXE")
	}
	cwd := t.TempDir()
	bin := t.TempDir()
	writeExecutable(t, interpreterFile(cwd))
	expected := interpreterFile(bin)
	writeExecutable(t, expected)
	t.Chdir(cwd)
	t.Setenv("PATH", "."+string(os.PathListSeparator)+bin)

	got, err := lookPathCommand("python")
	if err != nil {
		t.Fatalf("lookPathCommand: %v", err)
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(got, expected) {
			t.Fatalf("resolved = %q, want %q", got, expected)
		}
	} else if got != expected {
		t.Fatalf("resolved = %q, want %q", got, expected)
	}
}

func TestLookPathCommandAbsoluteAndMissing(t *testing.T) {
	path := interpreterFile(t.TempDir())
	writeExecutable(t, path)
	if got, err := lookPathCommand(path); err != nil || got != path {
		t.Fatalf("absolute = %q, %v", got, err)
	}
	if _, err := lookPathCommand(filepath.Join(t.TempDir(), "missing-python")); err == nil {
		t.Fatal("missing interpreter resolved")
	}
}

func TestResolveInterpreter(t *testing.T) {
	lookPath := func(name string) (string, error) {
		switch name {
		case "python3":
			return "/usr/bin/python3", nil
		case "custom":
			return "/opt/custom", nil
		default:
			return "", errors.New("not found")
		}
	}

	if got, found := resolveInterpreter("", lookPath); !found || got != "/usr/bin/python3" {
		t.Fatalf("auto = %q, %v", got, found)
	}
	if got, found := resolveInterpreter("custom", lookPath); !found || got != "/opt/custom" {
		t.Fatalf("configured = %q, %v", got, found)
	}
	if got, found := resolveInterpreter("missing", lookPath); found || got != "missing" {
		t.Fatalf("missing = %q, %v", got, found)
	}
}

func TestToolInfo(t *testing.T) {
	p := testPlugin(&fakeCaller{})
	info := p.toolInfo()
	if info.Capability != "python" || len(info.Tools) != 1 {
		t.Fatalf("info = %+v", info)
	}
	tool := info.Tools[0]
	if tool.Name != "python_execute" || tool.Method != "execute" {
		t.Fatalf("tool = %+v", tool)
	}
	if !strings.Contains(tool.Description, "python3") {
		t.Fatalf("description = %q", tool.Description)
	}
	p.interpreterFound = false
	if !strings.Contains(p.toolInfo().Tools[0].Description, "not found at startup") {
		t.Fatalf("description = %q", p.toolInfo().Tools[0].Description)
	}
}

func TestMergeMeta(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`{"exit_code":0}`)} {
		merged := mergeMeta(raw, map[string]any{"mode": "code"})
		var meta map[string]any
		if err := json.Unmarshal(merged, &meta); err != nil {
			t.Fatalf("merged meta: %v (%s)", err, merged)
		}
		if meta["mode"] != "code" {
			t.Fatalf("meta = %s", merged)
		}
	}
}
