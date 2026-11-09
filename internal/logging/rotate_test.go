package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterRotatesBySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	writer, err := New(path, 32, 2)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := writer.Write([]byte("0123456789\n")); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	main, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read main log: %v", err)
	}
	if len(main) > 32 {
		t.Fatalf("main log size = %d, want <= 32", len(main))
	}
	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("read rotated log: %v", err)
	}
	if len(rotated) == 0 {
		t.Fatal("rotated log is empty")
	}
	if !strings.HasSuffix(string(rotated), "\n") {
		t.Fatalf("rotated log lost line boundary: %q", rotated)
	}
}

func TestWriterReopensRotatedPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 64), 0o600); err != nil {
		t.Fatal(err)
	}
	writer, err := New(path, 32, 1)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer writer.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("active log size after startup rotation = %d, want 0", info.Size())
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("startup rotation missing backup: %v", err)
	}
}
