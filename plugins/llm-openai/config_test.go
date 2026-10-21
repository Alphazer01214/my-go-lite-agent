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

func keyPtr(s string) *string { return &s }

// chdirTemp runs a test in an empty plugin directory so loadConfig and
// saveConfig touch a throwaway config.json.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

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

func TestResolveAPIKey(t *testing.T) {
	old := []modelConfig{
		{ID: "a", APIKey: "sk-aaaaaaaaaa"},
		{ID: "b", APIKey: "sk-bbbbbbbbbb"},
	}
	if got := resolveAPIKey(nil, "a", old); got != "sk-aaaaaaaaaa" {
		t.Fatalf("absent keeps stored key: %q", got)
	}
	if got := resolveAPIKey(nil, "new", old); got != "" {
		t.Fatalf("absent on new id keeps nothing: %q", got)
	}
	if got := resolveAPIKey(keyPtr(""), "a", old); got != "" {
		t.Fatalf("empty clears: %q", got)
	}
	if got := resolveAPIKey(keyPtr("sk-newkeyhere"), "a", old); got != "sk-newkeyhere" {
		t.Fatalf("value replaces: %q", got)
	}
	if got := resolveAPIKey(keyPtr("sk-***aaa"), "a", old); got != "sk-aaaaaaaaaa" {
		t.Fatalf("masked echo keeps: %q", got)
	}
	// A renamed row round-trips its masked echo; the mask still matches.
	if got := resolveAPIKey(keyPtr("sk-***bbb"), "renamed", old); got != "sk-bbbbbbbbbb" {
		t.Fatalf("masked echo survives rename: %q", got)
	}
	if got := resolveAPIKey(keyPtr("sk-***zzz"), "a", old); got != "" {
		t.Fatalf("unmatched mask must not be stored: %q", got)
	}
}

func TestConfigSetImmediateWhenIdle(t *testing.T) {
	dir := chdirTemp(t)

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
	// The new entry inherits the previous default's endpoint.
	entry := findModel(p.snapshot().Models, "m2")
	if entry == nil || entry.BaseURL != defaultBaseURL || entry.APIKey != "sk-1234567890" {
		t.Fatalf("entry=%+v", entry)
	}
}

func TestConfigChangeDoesNotAffectCapturedSnapshot(t *testing.T) {
	chdirTemp(t)

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
	chdirTemp(t)

	p := newPlugin()
	_, _ = p.configSet(configSetIn{Model: "a"})
	_, _ = p.configSet(configSetIn{Model: "b", APIKey: "sk-zzzzzzzzzz"})
	c := p.snapshot()
	if c.Model != "b" || c.APIKey != "sk-zzzzzzzzzz" {
		t.Fatalf("config=%+v", c)
	}
}

// The legacy top-level endpoint fields describe the whole catalog on an old
// payload: they fill the entries that carry no endpoint of their own.
func TestConfigSetLegacyEndpointFillsCatalog(t *testing.T) {
	chdirTemp(t)

	p := newPlugin()
	out, err := p.configSet(configSetIn{
		BaseURL: "https://legacy.test/v1/",
		APIKey:  "sk-legacy-key",
		Models: &[]modelEntryIn{
			{ID: "one"},
			{ID: "two", BaseURL: "https://two.test/v1", APIKey: keyPtr("sk-two")},
		},
		DefaultModel: "one",
	})
	if err != nil {
		t.Fatal(err)
	}
	one := findModel(p.snapshot().Models, "one")
	two := findModel(p.snapshot().Models, "two")
	if one.BaseURL != "https://legacy.test/v1" || one.APIKey != "sk-legacy-key" {
		t.Fatalf("one=%+v", one)
	}
	if two.BaseURL != "https://two.test/v1" || two.APIKey != "sk-two" {
		t.Fatalf("two=%+v", two)
	}
	if out.BaseURL != one.BaseURL || out.APIKey != maskKey(one.APIKey) {
		t.Fatalf("mirror=%+v", out)
	}
}

func TestLoadConfigMigratesLegacyModel(t *testing.T) {
	chdirTemp(t)
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
	if c.Models[0].BaseURL != "https://example.test/v1" {
		t.Fatalf("legacy endpoint not migrated: %+v", c.Models[0])
	}
}

// A pre-multi-provider file has one endpoint and several models: every entry
// inherits the top-level base_url and api_key.
func TestLoadConfigMigratesLegacyEndpoint(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile(configFile, []byte(`{
		"base_url":"https://example.test/v1",
		"api_key":"sk-file-key",
		"models":[{"id":"small"},{"id":"large","base_url":"https://other.test/v1"}],
		"default_model":"small"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := loadConfig()
	small := findModel(c.Models, "small")
	large := findModel(c.Models, "large")
	if small.BaseURL != "https://example.test/v1" || small.APIKey != "sk-file-key" {
		t.Fatalf("small=%+v", small)
	}
	if large.BaseURL != "https://other.test/v1" || large.APIKey != "sk-file-key" {
		t.Fatalf("large=%+v", large)
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
	chdirTemp(t)
	t.Setenv("OPENAI_MODEL", "from-env")

	c := loadConfig()
	if c.DefaultModel != "from-env" || !hasModel(c.Models, "from-env") {
		t.Fatalf("config=%+v", c)
	}
}

// One env pair cannot describe several providers, so it addresses the default
// model's entry only.
func TestEnvEndpointTargetsDefaultModel(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile(configFile, []byte(`{
		"models":[
			{"id":"keep","base_url":"https://keep.test/v1","api_key":"sk-keep"},
			{"id":"moved","base_url":"https://moved.test/v1"}
		],
		"default_model":"moved"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_BASE_URL", "https://env.test/v1/")
	t.Setenv("OPENAI_API_KEY", "sk-env")

	c := loadConfig()
	moved := findModel(c.Models, "moved")
	keep := findModel(c.Models, "keep")
	if moved.BaseURL != "https://env.test/v1" || moved.APIKey != "sk-env" {
		t.Fatalf("moved=%+v", moved)
	}
	if keep.BaseURL != "https://keep.test/v1" || keep.APIKey != "sk-keep" {
		t.Fatalf("keep=%+v", keep)
	}
}

func TestModelsSetReplacesCatalog(t *testing.T) {
	dir := chdirTemp(t)

	p := newPlugin()
	out, err := p.modelsSet(modelsSetIn{
		Models: []modelEntryIn{
			{ID: "small", Label: "Small", ContextWindow: 8192, BaseURL: "https://small.test/v1", APIKey: keyPtr("sk-small")},
			{ID: "large", Label: "Large", ContextWindow: 131072, BaseURL: "https://large.test/v1", APIKey: keyPtr("sk-large")},
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
	if persisted.BaseURL != "" || persisted.APIKey != "" || persisted.Model != "" {
		t.Fatalf("legacy fields must not be written: %+v", persisted)
	}
	if len(persisted.Models) != 2 ||
		persisted.Models[0].BaseURL != "https://small.test/v1" || persisted.Models[0].APIKey != "sk-small" ||
		persisted.Models[1].BaseURL != "https://large.test/v1" || persisted.Models[1].APIKey != "sk-large" {
		t.Fatalf("persisted=%+v", persisted)
	}
	// llm.models.list never leaks endpoints or keys.
	if out.Models[0].ID != "small" {
		t.Fatalf("summary=%+v", out.Models[0])
	}
	if _, err := p.modelsSet(modelsSetIn{Models: []modelEntryIn{{ID: "a", BaseURL: "https://a.test/v1"}}, DefaultModel: "b"}); err == nil {
		t.Fatal("default outside catalog accepted")
	}
	if _, err := p.modelsSet(modelsSetIn{Models: []modelEntryIn{}}); err == nil {
		t.Fatal("empty catalog accepted")
	}
	if _, err := p.modelsSet(modelsSetIn{Models: []modelEntryIn{{ID: "a"}}}); err == nil {
		t.Fatal("entry without endpoint accepted")
	}
}

func TestModelsSetNoopKeepsExistingFile(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(configFile, []byte(`{
		"base_url":"https://example.test/v1",
		"api_key":"sk-test-file",
		"models":[{"id":"only","label":"Only","context_window":4096}],
		"default_model":"only"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newPlugin()
	// A metadata-only update keeps the stored endpoint and key: absent
	// base_url/api_key mean "unchanged".
	if _, err := p.modelsSet(modelsSetIn{
		Models:       []modelEntryIn{{ID: "only", Label: "Only", ContextWindow: 4096}},
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
	// The legacy file keeps its shape: a no-op save must not rewrite it.
	if persisted.APIKey != "sk-test-file" {
		t.Fatalf("no-op save rewrote the file: %+v", persisted)
	}
}

// The masked echo config.get hands out is accepted back unchanged; an empty
// string clears the key.
func TestModelsSetKeyRoundTrip(t *testing.T) {
	chdirTemp(t)

	p := newPlugin()
	in := modelsSetIn{
		Models:       []modelEntryIn{{ID: "m", BaseURL: "https://m.test/v1", APIKey: keyPtr("sk-secret123")}},
		DefaultModel: "m",
	}
	if _, err := p.modelsSet(in); err != nil {
		t.Fatal(err)
	}
	in.Models[0].APIKey = keyPtr(maskKey("sk-secret123"))
	if _, err := p.modelsSet(in); err != nil {
		t.Fatal(err)
	}
	if got := findModel(p.snapshot().Models, "m").APIKey; got != "sk-secret123" {
		t.Fatalf("masked round-trip lost the key: %q", got)
	}
	in.Models[0].APIKey = keyPtr("")
	if _, err := p.modelsSet(in); err != nil {
		t.Fatal(err)
	}
	if got := findModel(p.snapshot().Models, "m").APIKey; got != "" {
		t.Fatalf("empty did not clear: %q", got)
	}
}

func TestCompleteReportsResolvedModelAndWindow(t *testing.T) {
	chdirTemp(t)

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
		Models:       []modelEntryIn{{ID: "small", ContextWindow: 8192, BaseURL: server.URL}},
		DefaultModel: "small",
	}); err != nil {
		t.Fatal(err)
	}

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

// Two providers in one catalog: the model id picks the endpoint and the key.
func TestCompleteRoutesPerModelEndpoint(t *testing.T) {
	chdirTemp(t)

	type hit struct{ auth, model string }
	serve := func() (*httptest.Server, *hit) {
		h := &hit{}
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			h.auth = r.Header.Get("Authorization")
			h.model = body.Model
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
		}))
		t.Cleanup(s.Close)
		return s, h
	}
	first, hitA := serve()
	second, hitB := serve()

	p := newPlugin()
	if _, err := p.modelsSet(modelsSetIn{
		Models: []modelEntryIn{
			{ID: "model-a", BaseURL: first.URL, APIKey: keyPtr("sk-aaaaaa")},
			{ID: "model-b", BaseURL: second.URL, APIKey: keyPtr("sk-bbbbbb")},
		},
		DefaultModel: "model-a",
	}); err != nil {
		t.Fatal(err)
	}

	falseValue := false
	for _, tc := range []struct {
		model string
		hit   *hit
		auth  string
	}{{"model-a", hitA, "Bearer sk-aaaaaa"}, {"model-b", hitB, "Bearer sk-bbbbbb"}} {
		if _, err := p.complete(context.Background(), completeIn{
			Model:    tc.model,
			Messages: []chatMessage{{Role: "user", Content: "hi"}},
			Stream:   &falseValue,
		}, nil); err != nil {
			t.Fatal(err)
		}
		if tc.hit.model != tc.model || tc.hit.auth != tc.auth {
			t.Fatalf("%s hit=%+v want model=%s auth=%s", tc.model, *tc.hit, tc.model, tc.auth)
		}
	}
}

func TestCompleteRejectsUnknownModel(t *testing.T) {
	chdirTemp(t)

	p := newPlugin()
	_, err := p.complete(context.Background(), completeIn{
		Model:    "not-in-catalog",
		Messages: []chatMessage{{Role: "user", Content: "hi"}},
	}, nil)
	if err == nil {
		t.Fatal("unknown model accepted")
	}
}
