//go:build windows

package serve

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestJobCloseKillsAssignedProcess(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\nimport \"time\"\nfunc main(){ time.Sleep(2*time.Minute) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "sleeper.exe")
	if out, err := exec.Command("go", "build", "-o", exe, src).CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	job, err := newJob()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := job.assign(cmd); err != nil {
		t.Fatalf("assign: %v", err)
	}
	// Host death simulation: closing the job must kill the child.
	job.close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		killTree(cmd)
		t.Fatal("child survived job.close")
	}
}
