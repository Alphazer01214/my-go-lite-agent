package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	agentConfigPath      = "config.json"
	defaultAgentMaxSteps = 128
)

type config struct {
	MaxSteps int `json:"max_steps"`
}

func defaultConfig() config {
	return config{MaxSteps: defaultAgentMaxSteps}
}

func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, err
	}
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func (c config) validate() error {
	if c.MaxSteps <= 0 {
		return fmt.Errorf("max_steps must be positive")
	}
	return nil
}

type agentSettingsValues struct {
	MaxSteps int `json:"max_steps"`
}

type agentSettingsGetResp struct {
	Values agentSettingsValues `json:"values"`
}

type agentSettingsSaveReq struct {
	Values agentSettingsValues `json:"values"`
}

type settingsSaveResp struct {
	Saved bool `json:"saved"`
}

func (p *plugin) currentConfig() config {
	p.configMu.Lock()
	defer p.configMu.Unlock()
	return p.cfg
}

func (p *plugin) handleSettingsGet(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	cfg := p.currentConfig()
	return json.Marshal(agentSettingsGetResp{Values: agentSettingsValues{MaxSteps: cfg.MaxSteps}})
}

func (p *plugin) handleSettingsSave(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input agentSettingsSaveReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}
	cfg := config{MaxSteps: input.Values.MaxSteps}
	if err := cfg.validate(); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if err := saveConfig(agentConfigPath, cfg); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	p.configMu.Lock()
	p.cfg = cfg
	p.configMu.Unlock()
	return json.Marshal(settingsSaveResp{Saved: true})
}
