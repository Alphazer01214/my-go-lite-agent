package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type settingsValues struct {
	SkillsGlobalDir    string `json:"skills_global_dir"`
	SkillsWorkspaceDir string `json:"skills_workspace_dir"`
	MCPEnabled         bool   `json:"mcp_enabled"`
	MCPTimeoutMS       int    `json:"mcp_timeout_ms"`
}

type settingsGetResp struct {
	Values settingsValues `json:"values"`
}

type settingsSaveReq struct {
	Values settingsValues `json:"values"`
}

type settingsSaveResp struct {
	Saved bool `json:"saved"`
}

func (p *plugin) handleSettingsGet(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	cfg := p.currentConfig()
	return json.Marshal(settingsGetResp{Values: settingsValues{
		SkillsGlobalDir:    cfg.SkillsGlobalDir,
		SkillsWorkspaceDir: cfg.SkillsWorkspaceDir,
		MCPEnabled:         cfg.MCPEnabled,
		MCPTimeoutMS:       cfg.MCPTimeoutMS,
	}})
}

func (p *plugin) handleSettingsSave(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input settingsSaveReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}

	cfg := p.currentConfig()
	cfg.SkillsGlobalDir = strings.TrimSpace(input.Values.SkillsGlobalDir)
	cfg.SkillsWorkspaceDir = strings.TrimSpace(input.Values.SkillsWorkspaceDir)
	cfg.MCPEnabled = input.Values.MCPEnabled
	cfg.MCPTimeoutMS = input.Values.MCPTimeoutMS

	if cfg.SkillsGlobalDir != "" && !filepath.IsAbs(cfg.SkillsGlobalDir) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "skills_global_dir must be an absolute path")
	}
	if cfg.SkillsWorkspaceDir != "" && filepath.IsAbs(cfg.SkillsWorkspaceDir) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "skills_workspace_dir must be relative to the workspace")
	}
	if err := cfg.normalize(); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}

	path := p.configPath
	if path == "" {
		path = configPath
	}
	if err := saveConfig(path, cfg); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, fmt.Sprintf("save config: %v", err))
	}

	p.configMu.Lock()
	p.cfg = cfg
	p.configMu.Unlock()
	p.invalidateSkills()
	p.reloadMCP()

	return json.Marshal(settingsSaveResp{Saved: true})
}
