package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed config.example.json
var configExample []byte

func loadConfig(path string) (config, []diagnostic, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, []diagnostic{{
				Code:    "config_missing",
				Message: fmt.Sprintf("%s not found; no tool providers configured", path),
			}}, nil
		}
		return cfg, []diagnostic{{
			Code:    "config_invalid",
			Message: err.Error(),
		}}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultConfig(), []diagnostic{{
			Code:    "config_invalid",
			Message: err.Error(),
		}}, err
	}
	migrateLegacyConfig(&cfg)
	return cfg, cfg.normalize(), nil
}

// migrateLegacyConfig upgrades the original shipped default where tools only
// discovered file-tools. Runtime config is preserved across builds, so that
// exact historical default otherwise keeps newly shipped providers hidden.
func migrateLegacyConfig(cfg *config) {
	if len(cfg.Capabilities) != 1 || cfg.Capabilities[0] != "file" {
		return
	}
	if cfg.RefreshTTLMS != defaultRefreshTTLMS {
		return
	}
	if cfg.InfoTimeoutMS != defaultInfoTimeoutMS {
		return
	}
	var current config
	if err := json.Unmarshal(configExample, &current); err != nil {
		return
	}
	*cfg = current
}

func (c *config) normalize() []diagnostic {
	if c.RefreshTTLMS <= 0 {
		c.RefreshTTLMS = defaultRefreshTTLMS
	}
	if c.InfoTimeoutMS <= 0 {
		c.InfoTimeoutMS = defaultInfoTimeoutMS
	}

	seen := make(map[string]bool, len(c.Capabilities))
	capabilities := make([]string, 0, len(c.Capabilities))
	var diagnostics []diagnostic
	for _, raw := range c.Capabilities {
		capability := strings.TrimSpace(raw)
		if capability == "" {
			diagnostics = append(diagnostics, diagnostic{
				Code:    "config_invalid",
				Message: "empty capability is ignored",
			})
			continue
		}
		if !capabilityPattern.MatchString(capability) {
			diagnostics = append(diagnostics, diagnostic{
				Capability: capability,
				Code:       "config_invalid",
				Message:    "capability must match ^[a-z][a-z0-9-]{0,63}$",
			})
			continue
		}
		if reservedCapability(capability) {
			diagnostics = append(diagnostics, diagnostic{
				Capability: capability,
				Code:       "config_invalid",
				Message:    "reserved capability is not a tool provider",
			})
			continue
		}
		if seen[capability] {
			diagnostics = append(diagnostics, diagnostic{
				Capability: capability,
				Code:       "config_invalid",
				Message:    "duplicate capability is ignored",
			})
			continue
		}
		seen[capability] = true
		capabilities = append(capabilities, capability)
	}
	c.Capabilities = capabilities
	return diagnostics
}
