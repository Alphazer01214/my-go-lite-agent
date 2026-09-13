package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapabilityStarRouting(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer")
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("host invoke: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, `"via":"host"`) {
		t.Fatalf("want payload echoed through star: %s", s)
	}
	if strings.Contains(s, "direct") {
		t.Fatalf("must not use plugin-to-plugin direct channel: %s", s)
	}
}

func TestUnknownCapabilityStructuredError(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer", "-call-cap", "nope")
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want error for unknown capability: %s", out)
	}
	s := string(out)
	if !strings.Contains(s, "capability_unavailable") && !strings.Contains(s, "unknown capability") {
		t.Fatalf("want structured capability error: %s", s)
	}
}
