package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

func callTool(t *testing.T, workspace string, fn func(context.Context, *pluginsdk.Request, toolcontract.ProviderInput, json.RawMessage) (json.RawMessage, error), arguments string) toolcontract.Result {
	t.Helper()
	raw, err := fn(context.Background(), nil, toolcontract.ProviderInput{Workspace: workspace}, json.RawMessage(arguments))
	if err != nil {
		t.Fatal(err)
	}
	result, err := toolcontract.ParseResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestWriteReadReplacePatch(t *testing.T) {
	dir := t.TempDir()
	p := newPlugin()
	result := callTool(t, dir, p.write, `{"path":"sub/a.txt","content":"hello world\n","create_dirs":true}`)
	if result.IsError || result.Meta["created"] != true {
		t.Fatalf("write result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sub", "a.txt"))
	if err != nil || string(data) != "hello world\n" {
		t.Fatalf("file = %q, %v", data, err)
	}

	result = callTool(t, dir, p.read, `{"path":"sub/a.txt"}`)
	if result.IsError || result.Content != "hello world" {
		t.Fatalf("read = %+v", result)
	}
	sha, _ := result.Meta["sha256"].(string)
	if len(sha) != 64 {
		t.Fatalf("sha = %q", sha)
	}

	result = callTool(t, dir, p.replace, `{"path":"sub/a.txt","old_text":"world","new_text":"agent","expected_sha256":"`+sha+`"}`)
	if result.IsError {
		t.Fatalf("replace = %+v", result)
	}
	result = callTool(t, dir, p.patch, `{"path":"sub/a.txt","edits":[{"old_text":"hello","new_text":"hi"},{"old_text":"agent","new_text":"tools"}]}`)
	if result.IsError {
		t.Fatalf("patch = %+v", result)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "sub", "a.txt"))
	if string(data) != "hi tools\n" {
		t.Fatalf("patched = %q", data)
	}
}

func TestPatchFailureDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := callTool(t, dir, newPlugin().patch,
		`{"path":"a.txt","edits":[{"old_text":"before","new_text":"after"},{"old_text":"missing","new_text":"x"}]}`)
	if !result.IsError || result.Meta["code"] != "edit_not_found" {
		t.Fatalf("result = %+v", result)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(data) != "before" {
		t.Fatalf("file changed = %q", data)
	}
}

func TestSearchGlobListAndBinaryRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("package main\nfunc Alpha() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "src", "nested", "b.go"), []byte("// Alpha\npackage nested\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "image.bin"), []byte{0, 1, 2}, 0o644)

	p := newPlugin()
	result := callTool(t, dir, p.glob, `{"pattern":"**/*.go"}`)
	if result.Content != "src/a.go\nsrc/nested/b.go" {
		t.Fatalf("glob = %q", result.Content)
	}
	result = callTool(t, dir, p.search, `{"pattern":"Alpha","glob":"**/*.go"}`)
	if strings.Count(result.Content, "Alpha") != 2 {
		t.Fatalf("search = %q", result.Content)
	}
	result = callTool(t, dir, p.list, `{"path":".","recursive":true}`)
	if !strings.Contains(result.Content, "src/a.go") || !strings.Contains(result.Content, "image.bin") {
		t.Fatalf("list = %q", result.Content)
	}
	result = callTool(t, dir, p.read, `{"path":"image.bin"}`)
	if !result.IsError || result.Meta["code"] != "file_binary" {
		t.Fatalf("binary read = %+v", result)
	}
}

func TestWorkspaceBoundaries(t *testing.T) {
	dir := t.TempDir()
	p := newPlugin()
	for _, path := range []string{"../outside", "E:/outside", "/outside", `\\server\share`, "a/../../b"} {
		_, err := p.read(context.Background(), nil, toolcontract.ProviderInput{Workspace: dir},
			json.RawMessage(`{"path":`+strconv.Quote(path)+`}`))
		if err == nil {
			t.Fatalf("path %q was accepted", path)
		}
	}
	if _, err := p.read(context.Background(), nil, toolcontract.ProviderInput{}, json.RawMessage(`{"path":"a"}`)); err == nil {
		t.Fatal("missing workspace accepted")
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := p.read(context.Background(), nil, toolcontract.ProviderInput{Workspace: dir}, json.RawMessage(`{"path":"link/secret.txt"}`))
	if err == nil {
		t.Fatal("symlink escape accepted")
	}
}
