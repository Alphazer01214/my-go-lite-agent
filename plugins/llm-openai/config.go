package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
)

const configFile = "config.json"

// plugin holds the current config. complete captures a snapshot at call start.
type plugin struct {
	mu        sync.Mutex
	cfg       config
	active    map[string]context.CancelFunc
	cancelled map[string]bool
}

func newPlugin() *plugin {
	p := &plugin{active: make(map[string]context.CancelFunc), cancelled: make(map[string]bool)}
	p.cfg = loadConfig()
	return p
}

func loadConfig() config {
	cfg := config{
		BaseURL: "https://api.deepseek.com/v1",
		Model:   "deepseek-chat",
	}
	if data, err := os.ReadFile(configFile); err == nil {
		var c config
		if json.Unmarshal(data, &c) == nil {
			if c.BaseURL != "" {
				cfg.BaseURL = c.BaseURL
			}
			if c.APIKey != "" {
				cfg.APIKey = c.APIKey
			}
			if c.Model != "" {
				cfg.Model = c.Model
			}
		}
	}
	// env wins at startup
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		cfg.Model = v
	}
	return cfg
}

func (p *plugin) registerCall(callID string, cancel context.CancelFunc) {
	if callID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelled[callID] {
		cancel()
		return
	}
	p.active[callID] = cancel
}

func (p *plugin) cancelCall(callID string) bool {
	if callID == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if cancel, ok := p.active[callID]; ok {
		cancel()
		delete(p.active, callID)
		return true
	}
	p.cancelled[callID] = true
	return true
}

func (p *plugin) finishCall(callID string) {
	if callID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, callID)
	delete(p.cancelled, callID)
}
func (p *plugin) snapshot() config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *plugin) getConfig() configGetOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getConfigLocked()
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "***"
	}
	return k[:3] + "***" + k[len(k)-3:]
}

// configSet applies immediately. Running calls keep their captured snapshot.
func (p *plugin) configSet(in configSetIn) (configGetOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.applyLocked(in)
	return p.getConfigLocked(), nil
}

func (p *plugin) getConfigLocked() configGetOut {
	return configGetOut{
		BaseURL: p.cfg.BaseURL,
		APIKey:  maskKey(p.cfg.APIKey),
		Model:   p.cfg.Model,
	}
}

func (p *plugin) applyLocked(in configSetIn) {
	if in.BaseURL != "" {
		p.cfg.BaseURL = strings.TrimRight(in.BaseURL, "/")
	}
	if in.APIKey != "" {
		p.cfg.APIKey = in.APIKey
	}
	if in.Model != "" {
		p.cfg.Model = in.Model
	}
	_ = saveConfig(p.cfg)
}

func saveConfig(c config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0o644)
}
