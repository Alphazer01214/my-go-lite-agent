package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func settingsTestPlugin(t *testing.T) (*plugin, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		Skills:         []skillEntry{{Name: "manual-skill", Description: "manual"}},
		MCPServers:     map[string]mcpEntry{"manual-mcp": {Description: "manual"}},
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return &plugin{cfg: cfg, configPath: path}, path
}

func settingsRequest(t *testing.T, values settingsValues) *pluginsdk.Request {
	t.Helper()
	payload, err := json.Marshal(settingsSaveReq{Values: values})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	return &pluginsdk.Request{Payload: payload}
}

func TestSettingsSavePersistsAndPreservesConfig(t *testing.T) {
	p, path := settingsTestPlugin(t)
	ctx := context.Background()

	globalDir := filepath.Join(t.TempDir(), "skills")
	raw, err := p.handleSettingsSave(ctx, settingsRequest(t, settingsValues{
		SkillsGlobalDir:    globalDir,
		SkillsWorkspaceDir: ".liteagent/skills",
		MCPEnabled:         true,
		MCPTimeoutMS:       45000,
	}))
	if err != nil {
		t.Fatalf("save settings: %v", err)
	}
	var saved settingsSaveResp
	if err := json.Unmarshal(raw, &saved); err != nil || !saved.Saved {
		t.Fatalf("save response = %s, err=%v", raw, err)
	}

	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if len(loaded.PromptSections) != 1 || loaded.PromptSections[0].Name != "identity" {
		t.Fatalf("prompt sections lost: %+v", loaded.PromptSections)
	}
	if len(loaded.Skills) != 1 || loaded.Skills[0].Name != "manual-skill" {
		t.Fatalf("static skills lost: %+v", loaded.Skills)
	}
	if entry, ok := loaded.MCPServers["manual-mcp"]; !ok || entry.Name != "manual-mcp" {
		t.Fatalf("static mcp lost: %+v", loaded.MCPServers)
	}
	if loaded.SkillsGlobalDir != globalDir || loaded.SkillsWorkspaceDir != ".liteagent/skills" {
		t.Fatalf("skill dirs = %+v", loaded)
	}
	if !mcpGloballyEnabled(loaded) || loaded.MCPTimeoutMS != 45000 {
		t.Fatalf("mcp = %+v", loaded)
	}
	if p.currentConfig().SkillsGlobalDir != globalDir {
		t.Fatalf("hot reload failed: %+v", p.currentConfig())
	}

	getRaw, err := p.handleSettingsGet(ctx, &pluginsdk.Request{})
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	var got settingsGetResp
	if err := json.Unmarshal(getRaw, &got); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got.Values.SkillsGlobalDir != globalDir || !got.Values.MCPEnabled || got.Values.MCPTimeoutMS != 45000 {
		t.Fatalf("settings values = %+v", got.Values)
	}
}

func TestSettingsRejectsInvalidValues(t *testing.T) {
	p, _ := settingsTestPlugin(t)
	ctx := context.Background()

	for _, test := range []struct {
		name   string
		values settingsValues
	}{
		{
			name:   "relative global dir",
			values: settingsValues{SkillsGlobalDir: "relative/skills"},
		},
		{
			name:   "absolute workspace dir",
			values: settingsValues{SkillsWorkspaceDir: t.TempDir()},
		},
		{
			name:   "negative timeout",
			values: settingsValues{MCPTimeoutMS: -1},
		},
	} {
		_, err := p.handleSettingsSave(ctx, settingsRequest(t, test.values))
		if pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
			t.Fatalf("%s err = %v, want bad_arguments", test.name, err)
		}
	}
}

func TestConfigNormalizesMCPTimeout(t *testing.T) {
	cfg := config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
	}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.MCPTimeoutMS != defaultMCPTimeoutMS {
		t.Fatalf("timeout = %d, want %d", cfg.MCPTimeoutMS, defaultMCPTimeoutMS)
	}

	bad := config{
		PromptSections: []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		MCPTimeoutMS:   -1,
	}
	if err := bad.normalize(); err == nil {
		t.Fatal("negative timeout accepted")
	}
}

func TestLoadConfigMissingUsesEmbeddedDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.PromptSections) == 0 || cfg.MCPTimeoutMS != defaultMCPTimeoutMS || !mcpGloballyEnabled(cfg) {
		t.Fatalf("config = %+v", cfg)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("loadConfig created a file")
	}
}
