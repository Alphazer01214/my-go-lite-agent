package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoadConfigFallsBackToEmbeddedDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := loadConfig()
	if len(cfg.PromptSections) != 8 {
		t.Fatalf("fallback prompt sections = %d, want 8", len(cfg.PromptSections))
	}
	if cfg.PromptSections[0].Name != "identity" {
		t.Fatalf("first fallback section = %q, want identity", cfg.PromptSections[0].Name)
	}
}

func TestShippedConfigHasFullStaticPrompt(t *testing.T) {
	var raw config
	if err := json.Unmarshal(shippedConfig, &raw); err != nil {
		t.Fatalf("embedded config.example.json: %v", err)
	}
	cfg := normalizeConfig(raw)

	want := []struct {
		name  string
		order int
	}{
		{"identity", -1000},
		{"persona", -900},
		{"system_semantics", -800},
		{"task_policy", -700},
		{"tool_discipline", -600},
		{"communication", -500},
		{"output_format", -400},
		{"safety", -300},
	}
	if len(cfg.PromptSections) != len(want) {
		t.Fatalf("prompt sections = %d, want %d", len(cfg.PromptSections), len(want))
	}
	for i, section := range cfg.PromptSections {
		if section.Name != want[i].name || section.Order != want[i].order {
			t.Errorf("section %d = %s/%d, want %s/%d",
				i, section.Name, section.Order, want[i].name, want[i].order)
		}
		if strings.TrimSpace(section.Text) == "" {
			t.Errorf("section %s is empty", section.Name)
		}
		if strings.Contains(section.Text, "{{") {
			t.Errorf("section %s uses a dynamic variable; static track must stay workspace-independent", section.Name)
		}
	}
	if len(cfg.SubagentSections) == 0 {
		t.Fatal("shipped config has no subagent prompt")
	}
	for _, section := range cfg.SubagentSections {
		if strings.TrimSpace(section.Text) == "" {
			t.Errorf("subagent section %s is empty", section.Name)
		}
		if strings.Contains(section.Text, "{{") {
			t.Errorf("subagent section %s uses a dynamic variable", section.Name)
		}
	}
}
