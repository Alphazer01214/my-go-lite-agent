package main_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestREPLTwoTurnsSameSession: two lines in one -repl process share Session context.
func TestREPLTwoTurnsSameSession(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-repl",
	)
	cmd.Env = hostEnv(t)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	go func() {
		_, _ = stdin.Write([]byte("first-marker\r\nsecond-marker\r\n/exit\r\n"))
		_ = stdin.Close()
	}()

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			b.WriteString(sc.Text())
			b.WriteByte('\n')
		}
		done <- b.String()
	}()

	var out string
	select {
	case out = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("repl timeout")
	}

	if !strings.Contains(out, "You said: first-marker") {
		t.Fatalf("want first turn reply: %s", out)
	}
	if !strings.Contains(out, "You said: second-marker") {
		t.Fatalf("want second turn reply: %s", out)
	}
}
