package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartFailsWhenConsumesUnmet(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildConsumerPluginDir(t, root, pluginsDir, "consumer") // consumes echo, echo not mounted

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["consumer"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want fail-loud for unmet consumes: %s", out)
	}
	s := string(out)
	if !strings.Contains(s, "consumes") && !strings.Contains(s, "echo") {
		t.Fatalf("want unmet consume diagnosis: %s", s)
	}
}

func TestCallTimeout(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSlowPluginDir(t, root, pluginsDir, "slow")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["slow"]}`)

	start := time.Now()
	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-call-plugin", "slow")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want timeout error: %s", out)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout too slow: %s", time.Since(start))
	}
	if !strings.Contains(string(out), "timeout") {
		t.Fatalf("want timeout in output: %s", out)
	}
}

func TestCrashThenOnDemandRestart(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildCrashOncePluginDir(t, root, pluginsDir, "crashy")
	// echo for consumer if needed — only crashy here
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["crashy"]}`)

	// First call may hit crash; host should restart on demand and eventually succeed.
	// Crash-once exits on first process start, so first Call after mount may fail once
	// then succeed on the same host invocation if we call twice — use -call-plugin twice via script.
	// Host CLI: -call-plugin name sends one call. We invoke host once; Start launches plugin,
	// plugin crashes; on-demand restart on Call should bring it back and succeed.
	out, err := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-call-plugin", "crashy").CombinedOutput()
	if err != nil {
		t.Fatalf("want on-demand restart success: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("want ok after restart: %s", out)
	}
}

func TestHostExitClosesPlugins(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo"]}`)

	done := make(chan struct{})
	go func() {
		_ = exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-call-plugin", "echo").Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("host did not exit; possible orphan plugin hold")
	}
}
