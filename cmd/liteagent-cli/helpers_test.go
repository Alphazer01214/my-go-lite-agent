package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

var binCache sync.Map // pkg -> built exe path (per test-binary process)

// buildPkg builds a package once per test-binary process and shares the exe
// across tests: the seam suite rebuilds the same host/plugin binaries dozens
// of times, which pushed the package past go's default 10m timeout.
func buildPkg(t *testing.T, root, pkg string) string {
	t.Helper()
	if v, ok := binCache.Load(pkg); ok {
		return v.(string)
	}
	out := filepath.Join(os.TempDir(), fmt.Sprintf("la-test-%d-%s", os.Getpid(), filepath.Base(pkg)+".exe"))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	binCache.Store(pkg, out)
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hostEnv isolates the Session Plugin data dir per test (avoids shared ./sessions pollution).
func hostEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(), "SESSION_DATA_DIR="+t.TempDir())
}
