package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMaskKey(t *testing.T) {
	if maskKey("") != "" {
		t.Fatal("empty")
	}
	if maskKey("short") != "***" {
		t.Fatalf("short: %q", maskKey("short"))
	}
	got := maskKey("sk-abcdefghij")
	if got != "sk-***hij" {
		t.Fatalf("got %q", got)
	}
}

func TestConfigSetImmediateWhenIdle(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	out, err := p.configSet(configSetIn{Model: "m2", APIKey: "sk-1234567890"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "m2" || out.APIKey != "sk-***890" {
		t.Fatalf("out=%+v", out)
	}
	if p.snapshot().Model != "m2" {
		t.Fatal("not applied")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal("config not written")
	}
}

func TestConfigChangeDoesNotAffectCapturedSnapshot(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	captured := p.snapshot()
	out, err := p.configSet(configSetIn{Model: "busy-model", APIKey: "sk-zzzzzzzzzz"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "busy-model" || out.APIKey != "sk-***zzz" {
		t.Fatalf("out=%+v", out)
	}
	if captured.Model == "busy-model" {
		t.Fatal("captured snapshot must not change")
	}
	if p.snapshot().Model != "busy-model" {
		t.Fatal("subsequent snapshot must use new config")
	}
}

func TestConfigChangesApplyInOrder(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	_, _ = p.configSet(configSetIn{Model: "a"})
	_, _ = p.configSet(configSetIn{Model: "b", APIKey: "sk-zzzzzzzzzz"})
	c := p.snapshot()
	if c.Model != "b" || c.APIKey != "sk-zzzzzzzzzz" {
		t.Fatalf("config=%+v", c)
	}
}

func TestLoadConfigMigratesLegacyModel(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()
	if err := os.WriteFile(configFile, []byte(`{"base_url":"https://example.test/v1","model":"legacy"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := loadConfig()
	if c.DefaultModel != "legacy" || c.Model != "legacy" {
		t.Fatalf("default=%q model=%q", c.DefaultModel, c.Model)
	}
	if len(c.Models) != 1 || c.Models[0].ID != "legacy" || c.Models[0].Label != "legacy" {
		t.Fatalf("models=%+v", c.Models)
	}
}

func TestNormalizeConfigRepairsCatalog(t *testing.T) {
	c := normalizeConfig(config{
		Models: []modelConfig{
			{ID: " a "},
			{ID: "a", Label: "duplicate"},
			{ID: "b", Label: "B", ContextWindow: -1},
		},
		DefaultModel: "missing",
	})
	if len(c.Models) != 2 || c.Models[0].ID != "a" || c.Models[0].Label != "a" {
		t.Fatalf("models=%+v", c.Models)
	}
	if c.Models[1].ContextWindow != 0 {
		t.Fatalf("negative window not clamped: %+v", c.Models[1])
	}
	if c.DefaultModel != "a" || c.Model != "a" {
		t.Fatalf("default=%q model=%q", c.DefaultModel, c.Model)
	}
}

func TestEnvModelJoinsCatalog(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()
	t.Setenv("OPENAI_MODEL", "from-env")

	c := loadConfig()
	if c.DefaultModel != "from-env" || !hasModel(c.Models, "from-env") {
		t.Fatalf("config=%+v", c)
	}
}

func TestModelsSetReplacesCatalog(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	out, err := p.modelsSet(modelsSetIn{
		Models: []modelConfig{
			{ID: "small", Label: "Small", ContextWindow: 8192},
			{ID: "large", Label: "Large", ContextWindow: 131072},
		},
		DefaultModel: "large",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.DefaultModel != "large" || len(out.Models) != 2 {
		t.Fatalf("out=%+v", out)
	}
	if got := p.snapshot(); got.DefaultModel != "large" || len(got.Models) != 2 {
		t.Fatalf("config=%+v", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		t.Fatal(err)
	}
	var persisted config
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.APIKey != "" || persisted.Model != "large" || len(persisted.Models) != 2 {
		t.Fatalf("persisted=%+v", persisted)
	}
	if _, err := p.modelsSet(modelsSetIn{Models: []modelConfig{{ID: "a"}}, DefaultModel: "b"}); err == nil {
		t.Fatal("default outside catalog accepted")
	}
	if _, err := p.modelsSet(modelsSetIn{Models: []modelConfig{}}); err == nil {
		t.Fatal("empty catalog accepted")
	}
}

func TestModelsSetNoopKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()
	if err := os.WriteFile(configFile, []byte(`{
		"base_url":"https://example.test/v1",
		"api_key":"sk-test-file",
		"models":[{"id":"only","label":"Only","context_window":4096}],
		"default_model":"only"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newPlugin()
	if _, err := p.modelsSet(modelsSetIn{
		Models:       []modelConfig{{ID: "only", Label: "Only", ContextWindow: 4096}},
		DefaultModel: "only",
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		t.Fatal(err)
	}
	var persisted config
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.APIKey != "sk-test-file" {
		t.Fatalf("no-op save rewrote the file: %+v", persisted)
	}
}

func TestCompleteReportsResolvedModelAndWindow(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`))
	}))
	t.Cleanup(server.Close)

	p := newPlugin()
	if _, err := p.modelsSet(modelsSetIn{
		Models:       []modelConfig{{ID: "small", ContextWindow: 8192}},
		DefaultModel: "small",
	}); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.cfg.BaseURL = server.URL
	p.mu.Unlock()

	falseValue := false
	out, err := p.complete(context.Background(), completeIn{
		Messages: []chatMessage{{Role: "user", Content: "hi"}},
		Stream:   &falseValue,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "small" {
		t.Fatalf("upstream model=%q", gotModel)
	}
	if out.Model != "small" || out.ContextWindow != 8192 {
		t.Fatalf("out=%+v", out)
	}
	if out.Usage.TotalTokens != 10 {
		t.Fatalf("usage=%+v", out.Usage)
	}
}
