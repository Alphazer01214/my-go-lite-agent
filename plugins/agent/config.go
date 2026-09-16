package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// scheme is one named Agent Loop policy (ADR-0023). allowedTools nil = no name filter;
// empty slice = no external tools. No readOnly gate — the face is mount + names only.
type scheme struct {
	DependsPlugins []string  `json:"dependsPlugins,omitempty"`
	AllowedTools   *[]string `json:"allowedTools,omitempty"`
	MaxSteps       *int      `json:"maxSteps,omitempty"`
	RunSubagent    *bool     `json:"runSubagent,omitempty"`
	Todo           *bool     `json:"todo,omitempty"`
}

type agentConfig struct {
	DefaultScheme string            `json:"defaultScheme"`
	Schemes       map[string]scheme `json:"schemes"`
}

func defaultSchemes() map[string]scheme {
	chatTools := []string{"read_file", "grep", "glob"}
	off := false
	on := true
	chatSteps := 8
	return map[string]scheme{
		"chat": {
			DependsPlugins: []string{},
			AllowedTools:   &chatTools,
			MaxSteps:       &chatSteps,
			RunSubagent:    &off,
			Todo:           &off,
		},
		"tool_calling": {
			RunSubagent: &on,
			Todo:        &on,
		},
		"coding": {
			DependsPlugins: []string{
				"filetools", "shelltools", "sandbox",
				"skill-manager", "project-context", "webtools",
			},
			RunSubagent: &on,
			Todo:        &on,
		},
	}
}

func defaultConfig() agentConfig {
	return agentConfig{DefaultScheme: "tool_calling", Schemes: defaultSchemes()}
}

func configPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "config.json")
	}
	return "config.json"
}

func loadConfig() agentConfig {
	cfg := defaultConfig()
	raw, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	var onDisk agentConfig
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		return cfg
	}
	if onDisk.DefaultScheme != "" {
		cfg.DefaultScheme = onDisk.DefaultScheme
	}
	for name, sc := range onDisk.Schemes {
		base := cfg.Schemes[name]
		if onDisk.Schemes[name].DependsPlugins != nil {
			base.DependsPlugins = sc.DependsPlugins
		}
		if sc.AllowedTools != nil {
			base.AllowedTools = sc.AllowedTools
		}
		if sc.MaxSteps != nil {
			base.MaxSteps = sc.MaxSteps
		}
		if sc.RunSubagent != nil {
			base.RunSubagent = sc.RunSubagent
		}
		if sc.Todo != nil {
			base.Todo = sc.Todo
		}
		cfg.Schemes[name] = base
	}
	return cfg
}

func saveConfig(cfg agentConfig) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), raw, 0o600)
}

func (a *agent) activeScheme() (string, scheme) {
	cfg := loadConfig()
	name := cfg.DefaultScheme
	sc, ok := cfg.Schemes[name]
	if !ok {
		name = "tool_calling"
		sc = cfg.Schemes[name]
	}
	return name, sc
}

// ensureSchemePlugins mounts dependsPlugins via Host ensurePlugins (ADR-0023).
// Missing or failed mounts are loud: a scheme that names a plugin must have it.
func (a *agent) ensureSchemePlugins(names []string) error {
	if len(names) == 0 {
		return nil
	}
	raw, err := callJSON(a.s, "host", "ensurePlugins", map[string]any{"names": names})
	if err != nil {
		return err
	}
	var out struct {
		Missing []string `json:"missing"`
		Failed  []string `json:"failed"`
	}
	_ = json.Unmarshal(raw, &out)
	if len(out.Missing) > 0 || len(out.Failed) > 0 {
		return fmt.Errorf("ensurePlugins missing=%v failed=%v", out.Missing, out.Failed)
	}
	return nil
}

// filterTools applies scheme.allowedTools: nil = no filter, empty = no external tools.
func filterTools(schemas []toolSchema, sc scheme) []toolSchema {
	if sc.AllowedTools == nil {
		return schemas
	}
	allow := map[string]bool{}
	for _, n := range *sc.AllowedTools {
		allow[n] = true
	}
	var out []toolSchema
	for _, t := range schemas {
		if t.Name == "" {
			continue
		}
		if allow[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func schemeMaxSteps(sc scheme) int {
	if sc.MaxSteps != nil && *sc.MaxSteps > 0 {
		return *sc.MaxSteps
	}
	return MaxSteps
}

func schemeBool(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func handleConfigCap(method string, payload json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "reload":
		cfg := loadConfig()
		return json.Marshal(map[string]any{"ok": true, "defaultScheme": cfg.DefaultScheme})
	case "get":
		cfg := loadConfig()
		return json.Marshal(map[string]any{
			"defaultScheme": cfg.DefaultScheme,
			"schemes":       cfg.Schemes,
			"fields": []map[string]any{
				{"name": "defaultScheme", "value": cfg.DefaultScheme, "type": "string"},
			},
		})
	case "schema":
		names := make([]string, 0, len(loadConfig().Schemes))
		for n := range loadConfig().Schemes {
			names = append(names, n)
		}
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{
					"name": "defaultScheme", "type": "string",
					"description": "Active Agent Scheme (chat|tool_calling|coding|custom)",
					"default":     "tool_calling",
				},
			},
			"schemes": loadConfig().Schemes,
		})
	case "set":
		var in map[string]any
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		cfg := loadConfig()
		if raw, ok := in["defaultScheme"]; ok {
			name := fmt.Sprint(raw)
			if name == "" {
				return nil, &protocol.FrameError{Code: "bad_assignment", Message: "defaultScheme is empty"}
			}
			cfg.DefaultScheme = name
			if _, ok := cfg.Schemes[name]; !ok {
				cfg.Schemes[name] = scheme{}
			}
		}
		// Optional full scheme replace: schemes.<name> = {dependsPlugins,allowedTools,...}
		if raw, ok := in["schemes"]; ok {
			b, err := json.Marshal(raw)
			if err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
			var patch map[string]scheme
			if err := json.Unmarshal(b, &patch); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
			if cfg.Schemes == nil {
				cfg.Schemes = map[string]scheme{}
			}
			for name, sc := range patch {
				cfg.Schemes[name] = sc
			}
		}
		if err := saveConfig(cfg); err != nil {
			return nil, &protocol.FrameError{Code: "save_failed", Message: err.Error()}
		}
		return json.Marshal(map[string]any{"ok": true, "defaultScheme": cfg.DefaultScheme})
	default:
		return nil, &protocol.FrameError{Code: "unknown_method", Message: "config." + method}
	}
}

func handleConfigCommand(args string) (json.RawMessage, error) {
	cfg := loadConfig()
	fields := strings.Fields(args)
	if len(fields) == 0 || fields[0] == "get" {
		names := make([]string, 0, len(cfg.Schemes))
		for n := range cfg.Schemes {
			names = append(names, n)
		}
		text := fmt.Sprintf("defaultScheme=%s\nschemes=%s", cfg.DefaultScheme, strings.Join(names, ","))
		return json.Marshal(map[string]string{"text": text})
	}
	if fields[0] != "set" {
		return nil, &protocol.FrameError{
			Code:    "bad_command",
			Message: "usage: /agent config [get|set defaultScheme=name]",
		}
	}
	for _, kv := range fields[1:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k != "defaultScheme" || v == "" {
			return nil, &protocol.FrameError{
				Code:    "bad_assignment",
				Message: "usage: /agent config set defaultScheme=<name>",
			}
		}
		cfg.DefaultScheme = v
		if _, ok := cfg.Schemes[v]; !ok {
			cfg.Schemes[v] = scheme{}
		}
	}
	if err := saveConfig(cfg); err != nil {
		return nil, &protocol.FrameError{Code: "save_failed", Message: err.Error()}
	}
	return json.Marshal(map[string]string{"text": "defaultScheme=" + cfg.DefaultScheme})
}
