package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type resourcesListReq struct {
	Workspace string `json:"workspace,omitempty"`
}

type resourceSkill struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
	Path        string `json:"path,omitempty"`
	Enabled     bool   `json:"enabled"`
	Error       string `json:"error,omitempty"`
}

type resourceMCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type resourceMCPServer struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Transport   string            `json:"transport,omitempty"`
	Status      string            `json:"status"`
	Enabled     bool              `json:"enabled"`
	Tools       []resourceMCPTool `json:"tools,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
}

type resourceMCPView struct {
	Enabled   bool                `json:"enabled"`
	TimeoutMS int                 `json:"timeout_ms"`
	Servers   []resourceMCPServer `json:"servers"`
}

type resourcesResponse struct {
	UpdatedAt          string          `json:"updated_at"`
	Workspace          string          `json:"workspace,omitempty"`
	SkillsGlobalDir    string          `json:"skills_global_dir,omitempty"`
	SkillsWorkspaceDir string          `json:"skills_workspace_dir,omitempty"`
	Skills             []resourceSkill `json:"skills"`
	SkillErrors        []string        `json:"skill_errors,omitempty"`
	MCP                resourceMCPView `json:"mcp"`
}

func (p *plugin) handleResourcesRefresh(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	p.invalidateSkills()
	p.reloadMCP()
	return json.Marshal(map[string]bool{"refreshed": true})
}

func (p *plugin) handleResourcesList(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var input resourcesListReq
	if req != nil && len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &input); err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
	}

	cfg := p.currentConfig()
	scannedSkills, skillErrors := scanSkills(cfg, input.Workspace)
	return json.Marshal(p.resourcesView(cfg, input.Workspace, scannedSkills, skillErrors))
}

func (p *plugin) resourcesView(cfg config, workspace string, scannedSkills []skillInfo, skillErrors []string) resourcesResponse {
	skills := make([]resourceSkill, 0, len(scannedSkills))
	for _, skill := range scannedSkills {
		skills = append(skills, resourceSkill{
			Name:        skill.Name,
			Description: skill.Description,
			Source:      skill.Source,
			Path:        skill.Path,
			Enabled:     skill.Enabled,
			Error:       skill.Error,
		})
	}

	return resourcesResponse{
		UpdatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		Workspace:          workspace,
		SkillsGlobalDir:    cfg.SkillsGlobalDir,
		SkillsWorkspaceDir: cfg.SkillsWorkspaceDir,
		Skills:             skills,
		SkillErrors:        skillErrors,
		MCP:                p.resourceMCP(cfg),
	}
}
