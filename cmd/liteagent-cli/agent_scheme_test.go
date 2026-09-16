package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAgentConfigSchemeSwitch: /agent config set defaultScheme must persist
// and appear in config get (Host sends {command,args}, not {name,args}).
func TestAgentConfigSchemeSwitch(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	buildSessionPluginDir(t, root, pluginsDir, "session")
	// minimal llm so agent dependsOn soft-skips if missing; session is enough for /lp
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", "")

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-repl")
	cmd.Env = hostEnv(t)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	cmd.Stdout = &sb
	cmd.Stderr = &sb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	_, _ = stdin.Write([]byte("/agent config get\n"))
	_, _ = stdin.Write([]byte("/agent config set defaultScheme=chat\n"))
	_, _ = stdin.Write([]byte("/agent config get\n"))
	_, _ = stdin.Write([]byte("/exit\n"))
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("timeout waiting for repl:\n%s", sb.String())
	}
	out := sb.String()
	if !strings.Contains(out, "defaultScheme=") {
		t.Fatalf("want config get output:\n%s", out)
	}
	if !strings.Contains(out, "defaultScheme=chat") {
		t.Fatalf("want defaultScheme=chat after set:\n%s", out)
	}
	// disk config beside agent.exe
	cfgPath := filepath.Join(pluginsDir, "agent", "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("agent config.json: %v\n%s", err, out)
	}
	if !strings.Contains(string(raw), "chat") {
		t.Fatalf("want chat in config.json: %s\n%s", raw, out)
	}
}
