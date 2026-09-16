package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildSessionProbePluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./testdata/plugins/sessionprobe")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["demo"],
		"consumes": ["session"],
		"entry": "`+name+`.exe"
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestMultiSessionIsolation: two sessionIds keep separate logs and derive (in one Host process).
func TestMultiSessionIsolation(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildSessionProbePluginDir(t, root, pluginsDir, "sessionprobe")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","sessionprobe"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-invoke", "sessionprobe",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("multi session probe: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	// alpha payload must contain alpha marker and not beta; beta the reverse.
	// invoke ok payload={"alpha":[...],"beta":[...]}
	if !strings.Contains(s, "alpha-only-marker") {
		t.Fatalf("want alpha marker: %s", s)
	}
	if !strings.Contains(s, "beta-only-marker") {
		t.Fatalf("want beta marker: %s", s)
	}
	// Isolation: extract alpha messages array and ensure it lacks beta marker.
	// Fixture returns alpha messages then beta; check alpha block before beta block.
	alphaIdx := strings.Index(s, `"alpha":[`)
	betaIdx := strings.Index(s, `"beta":[`)
	if alphaIdx < 0 || betaIdx < 0 || alphaIdx >= betaIdx {
		t.Fatalf("want alpha before beta in payload: %s", s)
	}
	alphaBlock := s[alphaIdx:betaIdx]
	if strings.Contains(alphaBlock, "beta-only-marker") {
		t.Fatalf("alpha session leaked beta fact: %s", alphaBlock)
	}
	betaBlock := s[betaIdx:]
	if strings.Contains(betaBlock, "alpha-only-marker") {
		t.Fatalf("beta session leaked alpha fact: %s", betaBlock)
	}
}

// TestDefaultSessionBackwardCompatible: omitting sessionId uses the default session.
func TestDefaultSessionBackwardCompatible(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"type":"message","role":"user","content":"default-marker"}]`,
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("default session: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "default-marker") {
		t.Fatalf("want default marker in derive: %s", out)
	}
}
