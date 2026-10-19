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
	mu     sync.Mutex
	cfg    config
	active map[string]context.CancelFunc
	// cancelled remembers cancels that arrived before their completion
	// registered. Host does not order req and cancel on the wire, so this is a
	// real case; cancelledOrder keeps the buffer bounded.
	cancelled      map[string]bool
	cancelledOrder []string
}

// cancelledWindow bounds the pre-registration cancel buffer.
const cancelledWindow = 256

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
	cfg.BaseURL = normalizeBaseURL(cfg.BaseURL)
	return cfg
}

// normalizeBaseURL is applied on every input path (file, env, config.set) so
// the upstream URL is built the same way no matter where it came from.
func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
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

// cancelCall cancels one in-flight completion. It reports whether this call
// reached a live completion. An unknown id is parked in the bounded cancel
// buffer instead of being reported as cancelled: the completion may not have
// registered yet, and claiming success for a call that never existed would be
// a lie.
func (p *plugin) cancelCall(callID string) bool {
	if callID == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel, ok := p.active[callID]
	if !ok {
		p.rememberCancelLocked(callID)
		return false
	}
	cancel()
	delete(p.active, callID)
	return true
}

// rememberCancelLocked buffers one early cancel, evicting the oldest entry so a
// stream of cancels for calls that never start cannot grow the map forever.
func (p *plugin) rememberCancelLocked(callID string) {
	if p.cancelled[callID] {
		return
	}
	p.cancelled[callID] = true
	p.cancelledOrder = append(p.cancelledOrder, callID)
	for len(p.cancelledOrder) > cancelledWindow {
		oldest := p.cancelledOrder[0]
		p.cancelledOrder = p.cancelledOrder[1:]
		delete(p.cancelled, oldest)
	}
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
	if err := p.applyLocked(in); err != nil {
		return configGetOut{}, errHandler(err.Error())
	}
	return p.getConfigLocked(), nil
}

func (p *plugin) getConfigLocked() configGetOut {
	return configGetOut{
		BaseURL: p.cfg.BaseURL,
		APIKey:  maskKey(p.cfg.APIKey),
		Model:   p.cfg.Model,
	}
}

func (p *plugin) applyLocked(in configSetIn) error {
	if in.BaseURL != "" {
		p.cfg.BaseURL = normalizeBaseURL(in.BaseURL)
	}
	if in.APIKey != "" {
		p.cfg.APIKey = in.APIKey
	}
	if in.Model != "" {
		p.cfg.Model = in.Model
	}
	// The key is masked for callers but must never reach disk: config.json is a
	// plain file in the plugin directory. It stays in memory / the environment.
	return saveConfig(p.cfg)
}

func saveConfig(c config) error {
	persisted := c
	if persisted.APIKey != "" {
		persisted.APIKey = ""
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0o644)
}
