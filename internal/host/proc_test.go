package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEntryReturnsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.exe"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	// entry 无扩展名（源码 manifest 常见），产物是 .exe
	got, err := resolveEntry(dir, "agent")
	if err != nil {
		t.Fatalf("resolveEntry: %v", err)
	}
	if !filepath.IsAbs(got) {
		// 相对路径 + cmd.Dir 会让 Windows 按子进程 cwd 再解析一次 → 找不到
		t.Fatalf("resolveEntry returned relative path %q, want absolute", got)
	}
	if fi, err := os.Stat(got); err != nil || fi.IsDir() {
		t.Fatalf("resolved path not a file: %q (%v)", got, err)
	}
}

func TestResolveEntryPrefersExactName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveEntry(dir, "agent")
	if err != nil {
		t.Fatalf("resolveEntry: %v", err)
	}
	if filepath.Base(got) != "agent" {
		t.Fatalf("got %q, want exact-name match", got)
	}
}

func TestResolveEntryMissing(t *testing.T) {
	if _, err := resolveEntry(t.TempDir(), "ghost"); err == nil {
		t.Fatal("want error for missing entry")
	}
}
