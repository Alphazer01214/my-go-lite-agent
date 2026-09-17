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

func runREPLLines(t *testing.T, hostBin, pluginsDir, cfg string, lines []string) string {
	t.Helper()
	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-repl")
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
		_, _ = stdin.Write([]byte(strings.Join(lines, "\n") + "\n"))
		_ = stdin.Close()
	}()
	var b strings.Builder
	done := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			b.WriteString(sc.Text())
			b.WriteByte('\n')
		}
		done <- b.String()
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(30 * time.Second):
		t.Fatal("repl timeout")
		return ""
	}
}

func TestREPLHelpAndExit(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, cfg, []string{"/help", "/lp", "/exit"})
	for _, want := range []string{
		"Native commands:",
		"/help",
		"/lp",
		"/refresh",
		"/exit",
		"Plugins:",
		"session",
		"stubllm",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in help/lp output:\n%s", want, out)
		}
	}
	// Session Log export is a session plugin command, not a Host native.
	if strings.Contains(out, "/dump-trace") {
		t.Fatalf("/dump-trace must not appear as a Host native (use /session dump-trace):\n%s", out)
	}
}

func TestREPLUnknownCommandSuggests(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, cfg, []string{"/hlp", "/exit"})
	if !strings.Contains(out, "unknown command: hlp") {
		t.Fatalf("want unknown command: %s", out)
	}
	if !strings.Contains(out, "did you mean") {
		t.Fatalf("want suggestion: %s", out)
	}
}

func TestREPLSessionDumpTracePluginCommand(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [{"name":"identity","order":-1000,"text":"CMD_HELP_SYS"}]
	}`)
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","context-manager","agent"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, cfg, []string{
		"/session dump-trace",
		"/context-manager usage",
		"/context-manager skills",
		"/help session",
		"/exit",
	})
	if !strings.Contains(out, "facts") && !strings.Contains(out, "sessionId") {
		t.Fatalf("want /session dump-trace JSON: %s", out)
	}
	if !strings.Contains(out, "dump-trace") {
		t.Fatalf("want session command listed in /help session: %s", out)
	}
}

// TestContextManagerListModelContext: after a turn, /context-manager list shows messages.
func TestContextManagerListModelContext(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")
	buildContextManagerPluginDir(t, root, pluginsDir, "context-manager", `{
		"segments": [{"name":"identity","order":-1000,"text":"LIST_MC_SYS"}]
	}`)
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","stubllm","context-manager","agent"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, cfg, []string{
		"hello list mc",
		"/context-manager list",
		"/exit",
	})
	if !strings.Contains(out, "Model Context") {
		t.Fatalf("want Model Context header: %s", out)
	}
	if !strings.Contains(out, "system:") || !strings.Contains(out, "user:") {
		t.Fatalf("want role-prefixed messages: %s", out)
	}
}
