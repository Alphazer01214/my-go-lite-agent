package main

import (
	"strings"
	"testing"
)

func testVars() map[string]string {
	return map[string]string{
		"workspace": "E:/workspace",
		"model":     "model-a",
		"platform":  "windows",
		"date":      "2026-10-30",
	}
}

func TestDefaultConfigRendersTwoTracks(t *testing.T) {
	cfg, err := parseConfig(defaultConfigJSON)
	if err != nil {
		t.Fatalf("parse default config: %v", err)
	}
	staticText, err := cfg.renderStatic(testVars())
	if err != nil {
		t.Fatalf("render static: %v", err)
	}
	if !strings.Contains(staticText, "You are LiteAgent") {
		t.Fatalf("static text missing identity: %q", staticText)
	}
	if strings.Contains(staticText, "{{") {
		t.Fatalf("static text contains a variable: %q", staticText)
	}
	if strings.Contains(staticText, "E:/workspace") {
		t.Fatalf("static text leaked a runtime fact: %q", staticText)
	}

	runtimeText, err := cfg.renderRuntime(testVars())
	if err != nil {
		t.Fatalf("render runtime: %v", err)
	}
	for _, want := range []string{"<runtime_context>", "Workspace: E:/workspace", "Model: model-a", "Platform: windows", "Date: 2026-10-30", "</runtime_context>"} {
		if !strings.Contains(runtimeText, want) {
			t.Fatalf("runtime text missing %q: %q", want, runtimeText)
		}
	}
	if strings.Contains(runtimeText, "## Skills") || strings.Contains(runtimeText, "## MCP Servers") {
		t.Fatalf("empty skill/mcp lists should be omitted: %q", runtimeText)
	}
}

func TestRenderOrdersSectionsAndInterpolatesDynamicTrack(t *testing.T) {
	cfg := config{PromptSections: []promptSection{
		{Name: "second", Order: 50, Track: trackDynamic, Text: "workspace={{workspace}}"},
		{Name: "first", Order: -1000, Track: trackStatic, Text: "first"},
		{Name: "later", Order: -900, Track: trackStatic, Text: "later"},
	}}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	staticText, err := cfg.renderStatic(testVars())
	if err != nil {
		t.Fatalf("render static: %v", err)
	}
	if staticText != "first\n\nlater" {
		t.Fatalf("static text = %q", staticText)
	}
	runtimeText, err := cfg.renderRuntime(testVars())
	if err != nil {
		t.Fatalf("render runtime: %v", err)
	}
	if !strings.Contains(runtimeText, "workspace=E:/workspace") {
		t.Fatalf("dynamic interpolation missing: %q", runtimeText)
	}
}

func TestRenderSkillsAndMCP(t *testing.T) {
	cfg := config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		Skills: []skillEntry{
			{Name: "repo-search", Description: "Search the repository"},
			{Name: "planner"},
		},
		MCPServers: []mcpEntry{
			{Name: "filesystem", Description: "Local files", Tools: []string{"read", "write"}},
		},
	}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	runtimeText, err := cfg.renderRuntime(testVars())
	if err != nil {
		t.Fatalf("render runtime: %v", err)
	}
	for _, want := range []string{"## Skills", "- repo-search: Search the repository", "- planner", "## MCP Servers", "- filesystem: Local files (tools: read, write)"} {
		if !strings.Contains(runtimeText, want) {
			t.Fatalf("runtime text missing %q: %q", want, runtimeText)
		}
	}
}

func TestConfigRejectsBadSections(t *testing.T) {
	cases := []struct {
		name string
		cfg  config
		want string
	}{
		{
			name: "duplicate",
			cfg: config{PromptSections: []promptSection{
				{Name: "identity", Order: -1000, Text: "a"},
				{Name: "identity", Order: -900, Text: "b"},
			}},
			want: "prompt_section_duplicate",
		},
		{
			name: "static variable",
			cfg: config{PromptSections: []promptSection{
				{Name: "identity", Order: -1000, Track: trackStatic, Text: "{{workspace}}"},
			}},
			want: "prompt_variable_not_allowed",
		},
		{
			name: "unknown dynamic variable",
			cfg: config{PromptSections: []promptSection{
				{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"},
				{Name: "environment", Order: 0, Track: trackDynamic, Text: "{{unknown}}"},
			}},
			want: "prompt_variable_unknown",
		},
		{
			name: "empty static track",
			cfg: config{PromptSections: []promptSection{
				{Name: "environment", Order: 0, Track: trackDynamic, Text: "x"},
			}},
			want: "prompt_render_failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.normalize()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}
