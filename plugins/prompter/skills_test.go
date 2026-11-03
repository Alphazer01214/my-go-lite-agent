package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func writeSkill(t *testing.T, root, dir, name, description, body string) string {
	t.Helper()
	path := filepath.Join(root, dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	file := filepath.Join(path, "SKILL.md")
	content := body
	if name != "" || description != "" {
		content = "---\n"
		if name != "" {
			content += "name: " + name + "\n"
		}
		if description != "" {
			content += "description: " + description + "\n"
		}
		content += "---\n\n" + body
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return file
}

func skillsTestConfig(globalDir, workspaceDir string) config {
	return config{
		PromptSections:     []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		SkillsGlobalDir:    globalDir,
		SkillsWorkspaceDir: workspaceDir,
	}
}

func TestScanSkillsMergesGlobalAndWorkspace(t *testing.T) {
	globalDir := t.TempDir()
	workspace := t.TempDir()
	writeSkill(t, globalDir, "global-skill", "global-skill", "Global skill", "global body")
	writeSkill(t, globalDir, "shared", "shared", "Global shared", "global body")
	workspaceSkill := writeSkill(t, workspace, filepath.Join(".liteagent", "skills", "workspace-skill"), "", "", "Workspace skill body")
	writeSkill(t, workspace, filepath.Join(".liteagent", "skills", "shared"), "shared", "Workspace shared", "workspace body")

	cfg := skillsTestConfig(globalDir, ".liteagent/skills")
	skills, errors := scanSkills(cfg, workspace)
	if len(errors) != 0 {
		t.Fatalf("scan errors = %v", errors)
	}
	if len(skills) != 3 {
		t.Fatalf("skills = %+v", skills)
	}
	byName := make(map[string]skillInfo)
	for _, skill := range skills {
		byName[skill.Name] = skill
	}
	if byName["workspace-skill"].Source != skillSourceWorkspace || byName["workspace-skill"].Description != "Workspace skill body" {
		t.Fatalf("workspace skill = %+v", byName["workspace-skill"])
	}
	if byName["workspace-skill"].Path != workspaceSkill {
		t.Fatalf("workspace path = %q", byName["workspace-skill"].Path)
	}
	if byName["shared"].Source != skillSourceWorkspace || byName["shared"].Description != "Workspace shared" {
		t.Fatalf("workspace override = %+v", byName["shared"])
	}
	if byName["global-skill"].Source != skillSourceGlobal {
		t.Fatalf("global skill = %+v", byName["global-skill"])
	}
}

func TestParseSkillFileFallsBackToDirectoryAndFirstLine(t *testing.T) {
	info, err := parseSkillFile("fallback", []byte("# Heading\n\nfirst line\nsecond"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Name != "fallback" || info.Description != "Heading" {
		t.Fatalf("info = %+v", info)
	}
}

func TestParseSkillFileRejectsInvalidName(t *testing.T) {
	if _, err := parseSkillFile("dir", []byte("---\nname: bad/name\ndescription: x\n---\n")); err == nil {
		t.Fatal("invalid name accepted")
	}
}

func TestSkillCatalogUsesStaticFallback(t *testing.T) {
	cfg := skillsTestConfig("", "")
	cfg.Skills = []skillEntry{{Name: "manual", Description: "manual skill"}}
	p := &plugin{cfg: cfg}
	skills, errors := p.skillCatalog("", false)
	if len(errors) != 0 || len(skills) != 1 || skills[0].Name != "manual" || skills[0].Source != skillSourceConfig {
		t.Fatalf("skills = %+v errors=%v", skills, errors)
	}
}

func TestSkillReadReturnsContent(t *testing.T) {
	globalDir := t.TempDir()
	writeSkill(t, globalDir, "demo", "demo", "Demo skill", "full instructions\n")
	p := &plugin{cfg: skillsTestConfig(globalDir, "")}
	payload, _ := json.Marshal(map[string]any{
		"arguments": map[string]string{"name": "demo"},
		"workspace": globalDir,
	})
	raw, err := p.handleSkillRead(context.Background(), &pluginsdk.Request{Payload: payload})
	if err != nil {
		t.Fatalf("skill read: %v", err)
	}
	var result providerToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("result: %v", err)
	}
	if result.IsError || !strings.Contains(result.Content, "full instructions") {
		t.Fatalf("result = %+v", result)
	}
	var meta map[string]any
	if err := json.Unmarshal(result.Meta, &meta); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta["name"] != "demo" || meta["source"] != skillSourceGlobal {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestSkillReadMissing(t *testing.T) {
	p := &plugin{cfg: skillsTestConfig(t.TempDir(), "")}
	payload, _ := json.Marshal(map[string]any{"arguments": map[string]string{"name": "missing"}})
	_, err := p.handleSkillRead(context.Background(), &pluginsdk.Request{Payload: payload})
	if pluginsdk.Code(err) != codeSkillNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestSkillInfoOnlyWithDirectories(t *testing.T) {
	p := &plugin{cfg: skillsTestConfig("", "")}
	raw, err := p.handleSkillInfo(context.Background(), &pluginsdk.Request{Payload: []byte(`{}`)})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	var info providerInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("info decode: %v", err)
	}
	if info.Capability != "skill" || len(info.Tools) != 0 {
		t.Fatalf("info = %+v", info)
	}

	p.cfg = skillsTestConfig(t.TempDir(), "")
	raw, err = p.handleSkillInfo(context.Background(), &pluginsdk.Request{Payload: []byte(`{}`)})
	if err != nil {
		t.Fatalf("info with dir: %v", err)
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("info decode: %v", err)
	}
	if len(info.Tools) != 1 || info.Tools[0].Name != "skill_read" || info.Tools[0].Method != "read" {
		t.Fatalf("info = %+v", info)
	}
}

func TestBaselineFactsAddsReadableSkillHint(t *testing.T) {
	globalDir := t.TempDir()
	writeSkill(t, globalDir, "demo", "demo", "Demo skill", "body")
	p := &plugin{cfg: skillsTestConfig(globalDir, "")}
	facts, err := p.baselineFacts(t.TempDir(), "model-a")
	if err != nil {
		t.Fatalf("baselineFacts: %v", err)
	}
	if len(facts) != 2 || !strings.Contains(facts[1].TextContent, "## Skills") || !strings.Contains(facts[1].TextContent, "skill_read") {
		t.Fatalf("facts = %+v", facts)
	}
}
