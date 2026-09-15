package main_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequestHeaderSnapshot: each model hop logs a request_header fact; derive ignores it.
func TestRequestHeaderSnapshot(t *testing.T) {
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
		"-turn", "hello header",
		"-session-query",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("turn with request header: %v\n%s", err, out)
	}
	s := string(out)

	var facts []map[string]any
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "query ok facts=") {
			continue
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "query ok facts=")), &facts); err != nil {
			t.Fatalf("parse facts: %v\n%s", err, line)
		}
	}
	if len(facts) == 0 {
		t.Fatalf("want session facts: %s", s)
	}

	headers := 0
	for _, f := range facts {
		if f["type"] != "request_header" {
			continue
		}
		headers++
		meta, _ := f["meta"].(map[string]any)
		if meta == nil {
			t.Fatalf("request_header missing meta: %v", f)
		}
		if meta["provider"] == nil || meta["provider"] == "" {
			t.Fatalf("request_header missing provider: %v", f)
		}
		if meta["model"] == nil || meta["model"] == "" {
			t.Fatalf("request_header missing model: %v", f)
		}
	}
	if headers < 1 {
		t.Fatalf("want >=1 request_header fact: %s", s)
	}

	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "derive ok") {
			continue
		}
		if strings.Contains(line, "request_header") {
			t.Fatalf("derive must ignore request_header: %s", line)
		}
	}
}
