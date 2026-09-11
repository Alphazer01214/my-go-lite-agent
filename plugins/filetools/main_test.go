package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// helper: call a handler with JSON arguments and return the content string.
func callTool(t *testing.T, name string, args any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return dispatchTool(name, raw)
}

// assertFrameError checks that err is a FrameError with the given code.
func assertFrameError(t *testing.T, err error, wantCode string) {
	t.Helper()
	fe, ok := err.(*protocol.FrameError)
	if !ok {
		t.Fatalf("want FrameError, got %T: %v", err, err)
	}
	if fe.Code != wantCode {
		t.Fatalf("want error code %q, got %q: %s", wantCode, fe.Code, fe.Message)
	}
}

// ---------------------------------------------------------------------------
// read_file tests
// ---------------------------------------------------------------------------

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	out, err := callTool(t, "read_file", map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1\tline1") {
		t.Fatalf("want line 1 with number, got: %s", out)
	}
	if !strings.Contains(out, "3\tline3") {
		t.Fatalf("want line 3 with number, got: %s", out)
	}
}

func TestReadFileOffsetLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	var b strings.Builder
	for i := 1; i <= 10; i++ {
		b.WriteString("line" + strings.Repeat("x", i) + "\n")
	}
	os.WriteFile(path, []byte(b.String()), 0o644)

	out, err := callTool(t, "read_file", map[string]any{"path": path, "offset": 2, "limit": 3})
	if err != nil {
		t.Fatal(err)
	}
	// offset=2 means skip lines 0,1 (1-based lines 1,2), start at line 3
	if !strings.Contains(out, "3\t") {
		t.Fatalf("want line 3 in output, got: %s", out)
	}
	if !strings.Contains(out, "5\t") {
		t.Fatalf("want line 5 in output, got: %s", out)
	}
	if strings.Contains(out, "6\t") {
		t.Fatalf("must not contain line 6 (limit=3), got: %s", out)
	}
}

func TestReadFilePaginationHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	os.WriteFile(path, []byte(b.String()), 0o644)

	out, err := callTool(t, "read_file", map[string]any{"path": path, "offset": 0, "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "more lines") {
		t.Fatalf("want pagination hint in output, got: %s", out)
	}
	if !strings.Contains(out, "offset=10") {
		t.Fatalf("want offset hint, got: %s", out)
	}
}

func TestReadFileNotFound(t *testing.T) {
	_, err := callTool(t, "read_file", map[string]string{"path": "/nonexistent/file.txt"})
	assertFrameError(t, err, "file_error")
}

func TestReadFileMissingPath(t *testing.T) {
	_, err := callTool(t, "read_file", map[string]string{})
	assertFrameError(t, err, "bad_arguments")
}

// ---------------------------------------------------------------------------
// write_file tests
// ---------------------------------------------------------------------------

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	out, err := callTool(t, "write_file", map[string]string{"path": path, "content": "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "11 bytes") {
		t.Fatalf("want byte count in output, got: %s", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello world" {
		t.Fatalf("want file content %q, got %q", "hello world", string(got))
	}
}

func TestWriteFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.txt")

	_, err := callTool(t, "write_file", map[string]string{"path": path, "content": "nested"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "nested" {
		t.Fatalf("want %q, got %q", "nested", string(got))
	}
}

func TestWriteFileOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "over.txt")
	os.WriteFile(path, []byte("old"), 0o644)

	_, err := callTool(t, "write_file", map[string]string{"path": path, "content": "new"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("want %q after overwrite, got %q", "new", string(got))
	}
}

func TestWriteFileMissingPath(t *testing.T) {
	_, err := callTool(t, "write_file", map[string]string{"content": "x"})
	assertFrameError(t, err, "bad_arguments")
}

// ---------------------------------------------------------------------------
// edit_file tests
// ---------------------------------------------------------------------------

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world\nfoo bar\n"), 0o644)

	out, err := callTool(t, "edit_file", map[string]string{
		"path":       path,
		"old_string": "world",
		"new_string": "golang",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "edited") {
		t.Fatalf("want edited in output, got: %s", out)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "hello golang") {
		t.Fatalf("want edited content, got: %q", string(got))
	}
	if !strings.Contains(string(got), "foo bar") {
		t.Fatalf("want untouched line preserved, got: %q", string(got))
	}
}

func TestEditFileNotFound(t *testing.T) {
	_, err := callTool(t, "edit_file", map[string]string{
		"path":       "/nonexistent",
		"old_string": "a",
		"new_string": "b",
	})
	assertFrameError(t, err, "file_error")
}

func TestEditFileStringNotUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.txt")
	os.WriteFile(path, []byte("aa bb aa cc\n"), 0o644)

	_, err := callTool(t, "edit_file", map[string]string{
		"path":       path,
		"old_string": "aa",
		"new_string": "zz",
	})
	assertFrameError(t, err, "edit_failed")
}

func TestEditFileStringNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "miss.txt")
	os.WriteFile(path, []byte("nothing here\n"), 0o644)

	_, err := callTool(t, "edit_file", map[string]string{
		"path":       path,
		"old_string": "nope",
		"new_string": "zz",
	})
	assertFrameError(t, err, "edit_failed")
}

func TestEditFileMultilineReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	_, err := callTool(t, "edit_file", map[string]string{
		"path":       path,
		"old_string": "line1\nline2",
		"new_string": "replaced",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "replaced\nline3\n" {
		t.Fatalf("want multiline edit, got: %q", string(got))
	}
}

func TestEditFileMissingPath(t *testing.T) {
	_, err := callTool(t, "edit_file", map[string]string{"old_string": "a", "new_string": "b"})
	assertFrameError(t, err, "bad_arguments")
}

func TestEditFileEmptyOldString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	os.WriteFile(path, []byte("content"), 0o644)

	_, err := callTool(t, "edit_file", map[string]string{"path": path, "old_string": "", "new_string": "b"})
	assertFrameError(t, err, "bad_arguments")
}

// ---------------------------------------------------------------------------
// grep tests
// ---------------------------------------------------------------------------

func TestGrepBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\nfoo bar\nhello again\n"), 0o644)

	out, err := callTool(t, "grep", map[string]string{"pattern": "hello", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello world") {
		t.Fatalf("want first match, got: %s", out)
	}
	if !strings.Contains(out, "hello again") {
		t.Fatalf("want second match, got: %s", out)
	}
	if !strings.Contains(out, ":1:") {
		t.Fatalf("want line number 1, got: %s", out)
	}
	if !strings.Contains(out, ":3:") {
		t.Fatalf("want line number 3, got: %s", out)
	}
}

func TestGrepRegex(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "r.txt"), []byte("abc123\ndef456\nnope\n"), 0o644)

	out, err := callTool(t, "grep", map[string]string{"pattern": `\d+`, "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "nope") {
		t.Fatalf("must not match line without digits, got: %s", out)
	}
}

func TestGrepWithGlob(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "match.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("package skip\n"), 0o644)

	out, err := callTool(t, "grep", map[string]string{"pattern": "package", "path": dir, "glob": "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "match.go") {
		t.Fatalf("want .go file match, got: %s", out)
	}
	if strings.Contains(out, "skip.txt") {
		t.Fatalf("must not match .txt with glob *.go, got: %s", out)
	}
}

func TestGrepSkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".hidden")
	os.Mkdir(hidden, 0o755)
	os.WriteFile(filepath.Join(hidden, "secret.txt"), []byte("foundme\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.txt"), []byte("foundme\n"), 0o644)

	out, err := callTool(t, "grep", map[string]string{"pattern": "foundme", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, ".hidden") {
		t.Fatalf("must not search hidden dirs, got: %s", out)
	}
	if !strings.Contains(out, "visible.txt") {
		t.Fatalf("want visible match, got: %s", out)
	}
}

func TestGrepNoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte("nothing\n"), 0o644)

	out, err := callTool(t, "grep", map[string]string{"pattern": "zzzzz", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no matches") {
		t.Fatalf("want no matches message, got: %s", out)
	}
}

func TestGrepInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	_, err := callTool(t, "grep", map[string]string{"pattern": "[invalid", "path": dir})
	assertFrameError(t, err, "bad_arguments")
}

func TestGrepMissingPattern(t *testing.T) {
	dir := t.TempDir()
	_, err := callTool(t, "grep", map[string]string{"path": dir})
	assertFrameError(t, err, "bad_arguments")
}

// ---------------------------------------------------------------------------
// glob tests
// ---------------------------------------------------------------------------

func TestGlobBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644)

	out, err := callTool(t, "glob", map[string]string{"pattern": "*.go", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go") {
		t.Fatalf("want a.go, got: %s", out)
	}
	if !strings.Contains(out, "b.go") {
		t.Fatalf("want b.go, got: %s", out)
	}
	if strings.Contains(out, "c.txt") {
		t.Fatalf("must not contain c.txt, got: %s", out)
	}
}

func TestGlobRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "top.go"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(sub, "deep.go"), []byte(""), 0o644)

	out, err := callTool(t, "glob", map[string]string{"pattern": "**/*.go", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "top.go") {
		t.Fatalf("want top.go, got: %s", out)
	}
	if !strings.Contains(out, "deep.go") {
		t.Fatalf("want deep.go, got: %s", out)
	}
}

func TestGlobNoMatches(t *testing.T) {
	dir := t.TempDir()
	out, err := callTool(t, "glob", map[string]string{"pattern": "*.xyz", "path": dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no files") {
		t.Fatalf("want no files message, got: %s", out)
	}
}

func TestGlobMissingPattern(t *testing.T) {
	_, err := callTool(t, "glob", map[string]string{"path": "."})
	assertFrameError(t, err, "bad_arguments")
}

// ---------------------------------------------------------------------------
// tools.list test
// ---------------------------------------------------------------------------

func TestToolListCount(t *testing.T) {
	if len(toolSchemas) != 5 {
		t.Fatalf("want 5 tool schemas, got %d", len(toolSchemas))
	}
	names := map[string]bool{}
	for _, s := range toolSchemas {
		names[s["name"].(string)] = true
	}
	for _, want := range []string{"read_file", "write_file", "edit_file", "grep", "glob"} {
		if !names[want] {
			t.Fatalf("missing tool schema: %s", want)
		}
	}
}
