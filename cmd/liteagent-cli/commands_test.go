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
	hostBin := buildPkg(t, root, "./cmd/host")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, cfg, []string{"/help", "/lp", "/exit"})
	for _, want := range []string{
		"Native commands:",
		"/help",
		"/lp",
		"/refresh",
		"/dump-trace",
		"/exit",
		"Plugins:",
		"session",
		"fakellm",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in help/lp output:\n%s", want, out)
		}
	}
}

func TestREPLUnknownCommandSuggests(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	out := runREPLLines(t, hostBin, pluginsDir, cfg, []string{"/hlp", "/exit"})
	if !strings.Contains(out, "unknown command: hlp") {
		t.Fatalf("want unknown command: %s", out)
	}
	if !strings.Contains(out, "did you mean") {
		t.Fatalf("want suggestion: %s", out)
	}
}

func TestAssemblyRejectsNativeCommandConflict(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")
	pluginsDir := t.TempDir()
	// Fixture plugin named "lp" conflicts with native /lp.
	dir := filepath.Join(pluginsDir, "lp")
	writeFile(t, filepath.Join(dir, "plugin.json"), `{
		"name": "lp",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["echo"],
		"consumes": [],
		"entry": "lp.exe"
	}`)
	bin := buildPkg(t, root, "./plugins/echo")
	raw, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lp.exe"), raw, 0o755); err != nil {
		t.Fatal(err)
	}
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["lp","session","fakellm"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-repl")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	go func() {
		_, _ = stdin.Write([]byte("/exit\n"))
		_ = stdin.Close()
	}()
	var outB, errB strings.Builder
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			outB.WriteString(sc.Text())
			outB.WriteByte('\n')
		}
	}()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			errB.WriteString(sc.Text())
			errB.WriteByte('\n')
		}
	}()
	time.Sleep(3 * time.Second)
	combined := outB.String() + errB.String()
	if !strings.Contains(combined, "reject plugin lp") && !strings.Contains(combined, "conflicts with native command") {
		t.Fatalf("want rejection notice:\n%s", combined)
	}
}
