package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestSettingsMaskSecretAndPreserveOmittedSecret(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldDir)

	p := &plugin{cfg: config{
		DefaultModel: "model-a",
		Models: []ModelConfig{{
			ID:           "model-a",
			Label:        "Model A",
			BaseURL:      "https://example.test/v1",
			APIKey:       "secret-value",
			Capabilities: ModelCapabilities{Streaming: true, Tools: true},
		}},
	}}
	p.sdk = pluginsdk.NewPlugin("llm-openai")
	p.registerHandlers()
	registered := map[string]bool{}
	for _, item := range p.sdk.ListRegistered() {
		registered[item.Capability+"."+item.Method] = true
	}
	for _, key := range []string{"llm.models.list", "llm.settings.get", "llm.settings.save"} {
		if !registered[key] {
			t.Fatalf("missing registration %s", key)
		}
	}

	raw, err := p.handleSettingsGet(context.Background(), &pluginsdk.Request{})
	if err != nil {
		t.Fatalf("settings.get: %v", err)
	}
	if strings.Contains(string(raw), "secret-value") {
		t.Fatalf("settings.get leaked API key: %s", raw)
	}
	if !strings.Contains(string(raw), `"configured":true`) {
		t.Fatalf("settings.get missing configured state: %s", raw)
	}

	raw, err = p.handleSettingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"models":[{"id":"model-a","label":"Model A","base_url":"https://example.test/v1","streaming":true,"tools":true,"vision":true}],"default_model":"model-a"}}`),
	})
	if err != nil {
		t.Fatalf("settings.save: %v", err)
	}
	var saved settingsSaveResp
	if err := json.Unmarshal(raw, &saved); err != nil || !saved.Saved {
		t.Fatalf("save response = %s err = %v", raw, err)
	}
	if p.cfg.Models[0].APIKey != "secret-value" {
		t.Fatalf("API key = %q, want preserved value", p.cfg.Models[0].APIKey)
	}
	if !p.cfg.Models[0].Capabilities.Vision {
		t.Fatal("vision capability was not saved")
	}

	reloaded, err := loadConfig(llmConfigPath)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if reloaded.Models[0].APIKey != "secret-value" {
		t.Fatalf("persisted API key = %q, want preserved value", reloaded.Models[0].APIKey)
	}

	raw, err = p.handleModelsList(context.Background(), &pluginsdk.Request{})
	if err != nil {
		t.Fatalf("models.list: %v", err)
	}
	if strings.Contains(string(raw), "secret-value") || strings.Contains(string(raw), "api_key") {
		t.Fatalf("models.list leaked secret fields: %s", raw)
	}
}

func TestBuildChatRequestUsesSelectedModelAtTopLevel(t *testing.T) {
	request := buildChatRequest("selected-model", []pluginsdk.ChatMessage{{
		Role: pluginsdk.RoleUser,
		Text: "hello",
	}})
	if request.Model != "selected-model" {
		t.Fatalf("model = %q, want selected-model", request.Model)
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if strings.Contains(string(raw), "model=selected-model") {
		t.Fatalf("model leaked into messages: %s", raw)
	}
}

func TestConfigRejectsDuplicateModelIDs(t *testing.T) {
	cfg := config{
		DefaultModel: "model-a",
		Models: []ModelConfig{
			{ID: "model-a", BaseURL: "https://example.test/v1"},
			{ID: "model-a", BaseURL: "https://other.test/v1"},
		},
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected duplicate model id error")
	}
}

func TestConfigRequiresDefaultModel(t *testing.T) {
	cfg := config{Models: []ModelConfig{{ID: "model-a", BaseURL: "https://example.test/v1"}}}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected missing default_model error")
	}
}
