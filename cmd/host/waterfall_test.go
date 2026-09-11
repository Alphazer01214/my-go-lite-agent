package main_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWaterfallNoInterceptorAllows(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer", "-audit")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("no interceptor should allow: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, "audit from=consumer cap=echo") {
		t.Fatalf("want mandatory audit entry: %s", s)
	}
	if !strings.Contains(s, "action=allow") {
		t.Fatalf("want audit action=allow: %s", s)
	}
}

func TestWaterfallInterceptorAllow(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")
	buildInterceptorPluginDir(t, root, pluginsDir, "ix", "allow")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer","ix"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer", "-audit")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("allow interceptor: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, `"via":"host"`) {
		t.Fatalf("want original payload unchanged: %s", s)
	}
	if !strings.Contains(s, "action=allow") {
		t.Fatalf("want audit allow: %s", s)
	}
}

func TestWaterfallInterceptorRewrite(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")
	buildInterceptorPluginDir(t, root, pluginsDir, "ix", "rewrite")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer","ix"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer", "-audit")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rewrite interceptor: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, `"intercepted":true`) {
		t.Fatalf("want rewritten payload at downstream: %s", s)
	}
	if strings.Contains(s, `"via":"host"`) {
		t.Fatalf("payload must be rewritten before echo: %s", s)
	}
	if !strings.Contains(s, "action=rewrite") {
		t.Fatalf("want audit rewrite: %s", s)
	}
}

func TestWaterfallInterceptorReject(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")
	buildInterceptorPluginDir(t, root, pluginsDir, "ix", "reject")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer","ix"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer", "-audit")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want reject short-circuit: %s", out)
	}
	s := string(out)
	if !strings.Contains(s, "interceptor_rejected") && !strings.Contains(s, "policy_reject") {
		t.Fatalf("want interceptor rejection diagnosis: %s", s)
	}
	if !strings.Contains(s, "action=reject") {
		t.Fatalf("want audit reject: %s", s)
	}
}

func TestWaterfallMandatoryChainSurvivesInterceptorCrash(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildEchoPluginDir(t, root, pluginsDir, "echo")
	buildConsumerPluginDir(t, root, pluginsDir, "consumer")
	// Interceptor that exits immediately (crash) — mandatory chain + routing must remain.
	buildCrashInterceptorPluginDir(t, root, pluginsDir, "ix")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["echo","consumer","ix"]}`)

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-invoke", "consumer", "-audit")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("crashed interceptor must not block star routing: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok after interceptor crash: %s", s)
	}
	if !strings.Contains(s, "audit from=consumer cap=echo") {
		t.Fatalf("mandatory audit must still run: %s", s)
	}
	if !strings.Contains(s, "action=allow") {
		t.Fatalf("want fail-open allow after interceptor_down: %s", s)
	}
}

func TestWaterfallCoversAgentRequest(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildAgentProbePluginDir(t, root, pluginsDir, "agentprobe")

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","agentprobe"]}`)

	// Plugin-originated agent/request must still hit the mandatory Waterfall (audit).
	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-session-append", `[{"role":"user","content":"logged"}]`,
		"-invoke", "agentprobe",
		"-audit",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent/request through waterfall: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "invoke ok") {
		t.Fatalf("want invoke ok: %s", s)
	}
	if !strings.Contains(s, "audit from=agentprobe cap=agent method=request") {
		t.Fatalf("want mandatory audit on agent/request: %s", s)
	}
}
