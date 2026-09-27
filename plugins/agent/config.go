package main

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

const configFile = "config.json"

func defaultConfig() config {
	return config{DefaultScheme: schemeToolCalling, MaxSteps: 128}
}

type plugin struct {
	mu       sync.Mutex
	cfg      config
	working  int
	pending  *configSetIn
	sessions map[string]*sessionSlot
	sdk      sdkCaller
}

type sdkCaller interface {
	Call(capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallWithCallback(capability, method string, payload json.RawMessage, cb func(*event)) (json.RawMessage, error)
	EmitWithID(id string, capability string, method string, payload json.RawMessage) error
}

type sessionSlot struct {
	turnMu sync.Mutex // serializes turns on this session
	mu     sync.Mutex // protects active
	active *turnState
}

func newPlugin() *plugin {
	p := &plugin{sessions: map[string]*sessionSlot{}}
	p.cfg = loadConfig()
	return p
}

func loadConfig() config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg
	}
	var raw struct {
		DefaultScheme string `json:"default_scheme"`
		MaxSteps      int    `json:"max_steps"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}
	if raw.DefaultScheme != "" {
		cfg.DefaultScheme = raw.DefaultScheme
	}
	if raw.MaxSteps > 0 {
		cfg.MaxSteps = raw.MaxSteps
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
	return configGetOut{DefaultScheme: c.DefaultScheme, MaxSteps: c.MaxSteps}
}

func (p *plugin) beginWork() {
	p.mu.Lock()
	p.working++
	p.mu.Unlock()
}

func (p *plugin) endWork() {
	p.mu.Lock()
	if p.working > 0 {
		p.working--
	}
	if p.working == 0 && p.pending != nil {
		pending := *p.pending
		p.pending = nil
		p.applyLocked(pending)
	}
	p.mu.Unlock()
}

func (p *plugin) configSet(in configSetIn) (configGetOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.working > 0 {
		if p.pending == nil {
			p.pending = &configSetIn{}
		}
		if in.DefaultScheme != "" {
			p.pending.DefaultScheme = in.DefaultScheme
		}
		if in.MaxSteps > 0 {
			p.pending.MaxSteps = in.MaxSteps
		}
		out := configGetOut{DefaultScheme: p.cfg.DefaultScheme, MaxSteps: p.cfg.MaxSteps}
		if p.pending.DefaultScheme != "" {
			out.DefaultScheme = p.pending.DefaultScheme
		}
		if p.pending.MaxSteps > 0 {
			out.MaxSteps = p.pending.MaxSteps
		}
		return out, nil
	}
	p.applyLocked(in)
	return configGetOut{DefaultScheme: p.cfg.DefaultScheme, MaxSteps: p.cfg.MaxSteps}, nil
}

func (p *plugin) applyLocked(in configSetIn) {
	if in.DefaultScheme != "" {
		p.cfg.DefaultScheme = in.DefaultScheme
	}
	if in.MaxSteps > 0 {
		p.cfg.MaxSteps = in.MaxSteps
	}
	_ = os.WriteFile(configFile, mustJSON(p.cfg), 0o644)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

func (p *plugin) slotFor(id string) *sessionSlot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.sessions[id]
	if !ok {
		s = &sessionSlot{}
		p.sessions[id] = s
	}
	return s
}

// filterTools: allowed non-empty → intersection; else scheme.
func filterTools(tools []toolSpec, scheme string, allowed []string) []toolSpec {
	if len(allowed) > 0 {
		want := map[string]bool{}
		for _, n := range allowed {
			want[n] = true
		}
		out := make([]toolSpec, 0, len(tools))
		for _, t := range tools {
			if want[t.Name] {
				out = append(out, t)
			}
		}
		return out
	}
	if scheme == schemeChat {
		return nil
	}
	return tools
}

func maskScheme(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return schemeToolCalling
	}
	return s
}
