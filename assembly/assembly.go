// Package assembly decides which discovered Plugins are mounted.
package assembly

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/tomori/my-go-lite-agent/discovery"
)

// Config is the user-facing Assembly file (JSON).
type Config struct {
	Plugins []string `json:"plugins"`
}

// Plan is the resolved mount set against a Discovery result.
type Plan struct {
	Mounted   []discovery.Found
	Unmounted []discovery.Found
	Missing   []string
	// Rejected lists plugins that were discovered but refused (e.g. native Command name conflict).
	Rejected []Rejected
}

// Rejected is a Plugin excluded from mount and why.
type Rejected struct {
	Name   string
	Reason string
}

// Load reads an Assembly config file.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read assembly: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse assembly %s: %w", path, err)
	}
	return cfg, nil
}

// Resolve intersects cfg.Plugins with discovered plugins. Missing names stay in Plan.Missing.
// Plugins whose name conflicts with a Host-native Command are rejected (ADR-0008).
func Resolve(cfg Config, res discovery.Result) Plan {
	byName := make(map[string]discovery.Found, len(res.Plugins))
	for _, p := range res.Plugins {
		byName[p.Manifest.Name] = p
	}

	want := make(map[string]bool, len(cfg.Plugins))
	for _, name := range cfg.Plugins {
		want[name] = true
	}

	var plan Plan
	for _, name := range cfg.Plugins {
		p, ok := byName[name]
		if !ok {
			plan.Missing = append(plan.Missing, name)
			continue
		}
		if p.Manifest.ConflictsWithNativeCommand() {
			plan.Rejected = append(plan.Rejected, Rejected{
				Name:   name,
				Reason: fmt.Sprintf("plugin name %q conflicts with native command /%s", name, name),
			})
			continue
		}
		plan.Mounted = append(plan.Mounted, p)
	}
	for _, p := range res.Plugins {
		if !want[p.Manifest.Name] {
			plan.Unmounted = append(plan.Unmounted, p)
		}
	}
	sort.Slice(plan.Missing, func(i, j int) bool { return plan.Missing[i] < plan.Missing[j] })
	return plan
}
