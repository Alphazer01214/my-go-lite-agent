package main

import (
	"testing"
)

func TestResourcesViewListsSkillsAndMCP(t *testing.T) {
	globalDir := t.TempDir()
	writeSkill(t, globalDir, "repo-search", "repo-search", "Search", "body")
	p := &plugin{}
	cfg := config{
		PromptSections:     []promptSection{{Name: "identity", Order: -1000, Track: trackStatic, Text: "identity"}},
		SkillsGlobalDir:    globalDir,
		SkillsWorkspaceDir: ".liteagent/skills",
		MCPEnabled:         boolPtr(true),
		MCPTimeoutMS:       30000,
		MCPServers:         map[string]mcpEntry{"filesystem": {Description: "Files", Tools: []string{"read_file", "write_file"}}},
	}
	if err := cfg.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	skills, skillErrors := scanSkills(cfg, "/workspace")
	view := p.resourcesView(cfg, "/workspace", skills, skillErrors)
	if view.Workspace != "/workspace" {
		t.Fatalf("workspace = %q", view.Workspace)
	}
	if view.SkillsGlobalDir != globalDir || view.SkillsWorkspaceDir != ".liteagent/skills" {
		t.Fatalf("skill dirs = %+v", view)
	}
	if len(view.Skills) != 1 || view.Skills[0].Name != "repo-search" || view.Skills[0].Source != skillSourceGlobal {
		t.Fatalf("skills = %+v", view.Skills)
	}
	if !view.MCP.Enabled || view.MCP.TimeoutMS != 30000 {
		t.Fatalf("mcp = %+v", view.MCP)
	}
	if len(view.MCP.Servers) != 1 {
		t.Fatalf("servers = %+v", view.MCP.Servers)
	}
	server := view.MCP.Servers[0]
	if server.Name != "filesystem" || server.Status != mcpStatusUnconfigured || server.Transport != "stdio" || len(server.Tools) != 2 {
		t.Fatalf("server = %+v", server)
	}
}
