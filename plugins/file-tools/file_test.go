package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPlugin(t *testing.T) (*plugin, string) {
	t.Helper()
	return &plugin{}, t.TempDir()
}

func request(workspace string) *providerRequest {
	return &providerRequest{Workspace: workspace}
}

func resultCode(t *testing.T, result toolResult) string {
	t.Helper()
	var meta map[string]string
	if err := json.Unmarshal(result.Meta, &meta); err != nil {
		t.Fatalf("meta: %v (%s)", err, result.Meta)
	}
	return meta["code"]
}

func TestWriteReadEdit(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()

	written, err := p.write(ctx, request(workspace), &writeArgs{
		Path:    filepath.Join("sub", "note.txt"),
		Content: "hello\nworld\n",
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if written.IsError {
		t.Fatalf("write failed: %+v", written)
	}

	read, err := p.read(ctx, request(workspace), &readArgs{Path: "sub/note.txt"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.Content != "hello\nworld\n" {
		t.Fatalf("content = %q", read.Content)
	}

	edited, err := p.edit(ctx, request(workspace), &editArgs{
		Path:    "sub/note.txt",
		OldText: "world",
		NewText: "there",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if edited.IsError {
		t.Fatalf("edit failed: %+v", edited)
	}

	read, err = p.read(ctx, request(workspace), &readArgs{Path: "sub/note.txt", StartLine: 2, EndLine: 2})
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	if read.Content != "there\n" {
		t.Fatalf("range content = %q", read.Content)
	}
}

func TestReadLineRangeAndTruncation(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()
	if _, err := p.write(ctx, request(workspace), &writeArgs{
		Path:    "lines.txt",
		Content: "one\ntwo\nthree\n",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := p.read(ctx, request(workspace), &readArgs{Path: "lines.txt", StartLine: 2})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if result.Content != "two\nthree\n" {
		t.Fatalf("content = %q", result.Content)
	}

	result, err = p.read(ctx, request(workspace), &readArgs{Path: "lines.txt", MaxBytes: 5})
	if err != nil {
		t.Fatalf("read truncated: %v", err)
	}
	if result.Content != "one\n" || !result.Truncated {
		t.Fatalf("content=%q truncated=%v", result.Content, result.Truncated)
	}
}

func TestGlobAndGrep(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()
	for path, content := range map[string]string{
		"a.go":     "package main\n// needle\n",
		"sub/b.go": "package sub\n// needle\n",
		"sub/c.md": "needle\n",
	} {
		if _, err := p.write(ctx, request(workspace), &writeArgs{Path: path, Content: content}); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	globbed, err := p.glob(ctx, request(workspace), &globArgs{Pattern: "**/*.go"})
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if globbed.Content != "a.go\nsub/b.go" {
		t.Fatalf("glob content = %q", globbed.Content)
	}

	grep, err := p.grep(ctx, request(workspace), &grepArgs{Pattern: "needle", Path: ".", Glob: "**/*.go"})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(grep.Content, "a.go:2:4: // needle") || !strings.Contains(grep.Content, "sub/b.go:2:4: // needle") {
		t.Fatalf("grep content = %q", grep.Content)
	}
	if strings.Contains(grep.Content, "c.md") {
		t.Fatalf("grep should respect glob: %q", grep.Content)
	}
}

func TestEditAmbiguousAndMissing(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()
	if _, err := p.write(ctx, request(workspace), &writeArgs{
		Path:    "ambiguous.txt",
		Content: "x x",
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	result, err := p.edit(ctx, request(workspace), &editArgs{
		Path:    "ambiguous.txt",
		OldText: "x",
		NewText: "y",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !result.IsError || resultCode(t, result) != "edit_ambiguous" {
		t.Fatalf("result = %+v", result)
	}

	missing, err := p.edit(ctx, request(workspace), &editArgs{
		Path:    "missing.txt",
		OldText: "x",
		NewText: "y",
	})
	if err != nil {
		t.Fatalf("edit missing: %v", err)
	}
	if !missing.IsError || resultCode(t, missing) != "path_not_found" {
		t.Fatalf("missing = %+v", missing)
	}
}

func TestWriteCreateDirsFalse(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()
	disabled := false
	result, err := p.write(ctx, request(workspace), &writeArgs{
		Path:       filepath.Join("missing", "note.txt"),
		Content:    "x",
		CreateDirs: &disabled,
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !result.IsError || resultCode(t, result) != "path_not_found" {
		t.Fatalf("result = %+v", result)
	}
}

func TestPathEscapeAndWorkspace(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()

	if _, err := p.read(ctx, request(workspace), &readArgs{Path: "../outside.txt"}); err == nil {
		t.Fatal("path escape should be an error")
	}
	if _, err := p.read(ctx, &providerRequest{}, &readArgs{Path: "a.txt"}); err == nil {
		t.Fatal("missing workspace should be an error")
	}
}

func TestBinaryRead(t *testing.T) {
	p, workspace := testPlugin(t)
	ctx := context.Background()
	path := filepath.Join(workspace, "binary.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	result, err := p.read(ctx, request(workspace), &readArgs{Path: "binary.bin"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !result.IsError || resultCode(t, result) != "file_binary" {
		t.Fatalf("result = %+v", result)
	}
}
