package main

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

const configFile = "config.json"

// plugin holds config + working-state for deferred config.set.
type plugin struct {
	mu       sync.Mutex
	cfg      config
	working  int
	pending  *configSetIn // merged sets while working
}

func newPlugin() *plugin {
	p := &plugin{}
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

func (p *plugin) snapshot() config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *plugin) getConfig() configGetOut {
	c := p.snapshot()
	return configGetOut{
		BaseURL: c.BaseURL,
		APIKey:  maskKey(c.APIKey),
		Model:   c.Model,
	}
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

// beginWork marks a complete in-flight (working state).
func (p *plugin) beginWork() {
	p.mu.Lock()
	p.working++
	p.mu.Unlock()
}

// endWork drops working count and flushes deferred config.set if idle.
func (p *plugin) endWork() {
	p.mu.Lock()
	if p.working > 0 {
		p.working--
	}
	if p.working == 0 && p.pending != nil {
		pending := p.pending
		p.pending = nil
		p.applyLocked(*pending)
	}
	p.mu.Unlock()
}

// configSet applies immediately when idle; defers while working.
func (p *plugin) configSet(in configSetIn) (configGetOut, error) {
	p.mu.Lock()
	if p.working > 0 {
		if p.pending == nil {
			p.pending = &configSetIn{}
		}
		if in.BaseURL != "" {
			p.pending.BaseURL = in.BaseURL
		}
		if in.APIKey != "" {
			p.pending.APIKey = in.APIKey
		}
		if in.Model != "" {
			p.pending.Model = in.Model
		}
		// report live (pre-apply) values for base/model; key stays masked old/new after flush
		out := configGetOut{
			BaseURL: p.cfg.BaseURL,
			APIKey:  maskKey(p.cfg.APIKey),
			Model:   p.cfg.Model,
		}
		if p.pending.BaseURL != "" {
			out.BaseURL = p.pending.BaseURL
		}
		if p.pending.Model != "" {
			out.Model = p.pending.Model
		}
		if p.pending.APIKey != "" {
			out.APIKey = maskKey(p.pending.APIKey)
		}
		p.mu.Unlock()
		return out, nil
	}
	p.applyLocked(in)
	out := configGetOut{
		BaseURL: p.cfg.BaseURL,
		APIKey:  maskKey(p.cfg.APIKey),
		Model:   p.cfg.Model,
	}
	p.mu.Unlock()
	return out, nil
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
