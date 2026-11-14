package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	agentConfigPath      = "config.json"
	defaultAgentMaxSteps = 128

	toolParallelismSerial = "serial"
	toolParallelismAuto   = "auto"

	defaultMaxParallelTools = 4
	maxAllowedParallelTools = 8
)

var defaultParallelSafeTools = []string{"file_read", "file_glob", "file_grep", "subagent"}

type config struct {
	MaxSteps          int      `json:"max_steps"`
	ToolParallelism   string   `json:"tool_parallelism,omitempty"`
	MaxParallelTools  int      `json:"max_parallel_tools,omitempty"`
	ParallelSafeTools []string `json:"parallel_safe_tools,omitempty"`
}

func defaultConfig() config {
	return config{
		MaxSteps:          defaultAgentMaxSteps,
		ToolParallelism:   toolParallelismAuto,
		MaxParallelTools:  defaultMaxParallelTools,
		ParallelSafeTools: append([]string(nil), defaultParallelSafeTools...),
	}
}

func loadConfig(path string) (config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, err
	}
	cfg.normalize()
	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func saveConfig(path string, cfg config) error {
	cfg.normalize()
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

func (c *config) normalize() {
	if c.ToolParallelism == "" {
		c.ToolParallelism = toolParallelismAuto
	}
	if c.MaxParallelTools == 0 {
		c.MaxParallelTools = defaultMaxParallelTools
	}
	if len(c.ParallelSafeTools) == 0 {
		c.ParallelSafeTools = append([]string(nil), defaultParallelSafeTools...)
	}
}

func (c config) validate() error {
	if c.MaxSteps <= 0 {
		return fmt.Errorf("max_steps must be positive")
	}
	switch c.ToolParallelism {
	case toolParallelismSerial, toolParallelismAuto:
	default:
		return fmt.Errorf("tool_parallelism must be %q or %q", toolParallelismSerial, toolParallelismAuto)
	}
	if c.MaxParallelTools < 1 || c.MaxParallelTools > maxAllowedParallelTools {
		return fmt.Errorf("max_parallel_tools must be between 1 and %d", maxAllowedParallelTools)
	}
	seen := make(map[string]bool, len(c.ParallelSafeTools))
	for _, name := range c.ParallelSafeTools {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("parallel_safe_tools must not contain empty names")
		}
		if seen[name] {
			return fmt.Errorf("parallel_safe_tools contains duplicate %q", name)
		}
		seen[name] = true
	}
	return nil
}

func (c config) parallelEnabled() bool {
	return c.ToolParallelism == toolParallelismAuto && c.MaxParallelTools > 1
}

func (c config) safeToolSet() map[string]bool {
	names := c.ParallelSafeTools
	if len(names) == 0 {
		names = defaultParallelSafeTools
	}
	safe := make(map[string]bool, len(names))
	for _, name := range names {
		safe[name] = true
	}
	return safe
}

type agentSettingsValues struct {
	MaxSteps          int      `json:"max_steps"`
	ToolParallelism   string   `json:"tool_parallelism,omitempty"`
	MaxParallelTools  int      `json:"max_parallel_tools,omitempty"`
	ParallelSafeTools []string `json:"parallel_safe_tools,omitempty"`
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
	return json.Marshal(agentSettingsGetResp{Values: agentSettingsValues{
		MaxSteps:          cfg.MaxSteps,
		ToolParallelism:   cfg.ToolParallelism,
		MaxParallelTools:  cfg.MaxParallelTools,
		ParallelSafeTools: append([]string(nil), cfg.ParallelSafeTools...),
	}})
}

func (p *plugin) handleSettingsSave(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input agentSettingsSaveReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}
	cfg := p.currentConfig()
	if input.Values.MaxSteps <= 0 {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "max_steps must be positive")
	}
	cfg.MaxSteps = input.Values.MaxSteps
	if mode := strings.TrimSpace(input.Values.ToolParallelism); mode != "" {
		cfg.ToolParallelism = mode
	}
	if input.Values.MaxParallelTools != 0 {
		cfg.MaxParallelTools = input.Values.MaxParallelTools
	}
	if input.Values.ParallelSafeTools != nil {
		cfg.ParallelSafeTools = append([]string(nil), input.Values.ParallelSafeTools...)
	}
	cfg.normalize()
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
