package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

const configFile = "config.json"

func defaultConfig() config {
	return config{
		DefaultScheme: schemeToolCalling, MaxSteps: 128,
		ToolParallelism: "serial", MaxParallelTools: 4,
	}
}

type plugin struct {
	mu       sync.Mutex
	cfg      config
	sessions map[string]*sessionSlot
	sdk      sdkCaller
	turnSeq  atomic.Int64
}

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallWithCallback(ctx context.Context, capability, method string, payload json.RawMessage, cb func(*event)) (json.RawMessage, error)
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
		DefaultScheme    string `json:"default_scheme"`
		MaxSteps         int    `json:"max_steps"`
		ToolParallelism  string `json:"tool_parallelism"`
		MaxParallelTools int    `json:"max_parallel_tools"`
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
	if raw.ToolParallelism == "serial" || raw.ToolParallelism == "auto" {
		cfg.ToolParallelism = raw.ToolParallelism
	}
	if raw.MaxParallelTools >= 1 && raw.MaxParallelTools <= 16 {
		cfg.MaxParallelTools = raw.MaxParallelTools
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

// configSet applies immediately. Running turns keep their captured snapshot.
func (p *plugin) configSet(in configSetIn) (configGetOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.applyLocked(in); err != nil {
		return configGetOut{}, errHandler(err.Error())
	}
	return p.getConfigLocked(), nil
}

func (p *plugin) getConfigLocked() configGetOut {
	return configGetOut{DefaultScheme: p.cfg.DefaultScheme, MaxSteps: p.cfg.MaxSteps}
}

func (p *plugin) applyLocked(in configSetIn) error {
	if in.DefaultScheme != "" {
		p.cfg.DefaultScheme = in.DefaultScheme
	}
	if in.MaxSteps > 0 {
		p.cfg.MaxSteps = in.MaxSteps
	}
	// A failed write must not be reported as a save: the caller would believe
	// the new values survived a restart when they did not.
	return os.WriteFile(configFile, mustJSON(p.cfg), 0o644)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// slotFor returns the per-session turn lock, creating it on first use. Slots are
// kept for the process lifetime: one per session id the agent has ever served,
// which is bounded by the number of live sessions.
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

func sameWorkspacePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(leftAbs); err == nil {
		leftAbs = resolved
	}
	if resolved, err := filepath.EvalSymlinks(rightAbs); err == nil {
		rightAbs = resolved
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftAbs, rightAbs)
	}
	return leftAbs == rightAbs
}
