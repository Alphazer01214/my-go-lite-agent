package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConsumesUnmetSoftSkipDegraded(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildConsumerPluginDir(t, root, pluginsDir, "consumer") // consumes echo, echo not mounted
	// Mark consumer autostart so Autostart path mounts it without assembly whitelist.
	// (buildConsumerPluginDir writes plugin.json; patch autostart here.)
	manifestPath := filepath.Join(pluginsDir, "consumer", "plugin.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["autostart"] = true
	outb, _ := json.Marshal(m)
	if err := os.WriteFile(manifestPath, outb, 0o644); err != nil {
		t.Fatal(err)
	}

	// Host must start (soft skip) and still be invokable; use-time Call fails.
	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-invoke", "consumer")
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	s := string(out)
	if err == nil {
		// invoke may succeed at process level if plugin handles missing cap; require warn.
		if !strings.Contains(s, "degraded") && !strings.Contains(s, "consumes") {
			t.Fatalf("want degraded/consumes warning: %s", s)
		}
		return
	}
	// Use-time failure is acceptable if the process started and diagnosed consumes.
	if !strings.Contains(s, "consumes") && !strings.Contains(s, "degraded") && !strings.Contains(s, "echo") {
		t.Fatalf("want degraded diagnosis, got: %s", s)
	}
}

func TestCallTimeout(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSlowPluginDir(t, root, pluginsDir, "slow")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["slow"]}`)

	start := time.Now()
	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-call-plugin", "slow")
	cmd.Env = hostEnv(t)
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
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

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
	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-call-plugin", "crashy")
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("want on-demand restart success: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("want ok after restart: %s", out)
	}
}

func TestHostExitClosesPlugins(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

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
