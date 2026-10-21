package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type instanceSnapshot struct {
	State         string                 `json:"state"`
	Generation    int                    `json:"generation"`
	Registrations []registrationSnapshot `json:"registrations"`
}

type registrationSnapshot struct {
	Capability string `json:"capability"`
	Method     string `json:"method"`
}

type pluginSnapshot struct {
	Name      string             `json:"name"`
	State     string             `json:"state"`
	Enabled   bool               `json:"enabled"`
	Provides  []string           `json:"provides"`
	Instances []instanceSnapshot `json:"instances"`
}

type hostSnapshot struct {
	Plugins []pluginSnapshot `json:"plugins"`
}

func (p *toolsPlugin) hostPlugins(ctx context.Context) (hostSnapshot, error) {
	raw, err := p.sdk.Call(ctx, "host", "plugins", json.RawMessage(`{}`))
	if err != nil {
		return hostSnapshot{}, err
	}
	var snapshot hostSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return hostSnapshot{}, fmt.Errorf("decode host.plugins: %w", err)
	}
	return snapshot, nil
}

func (s hostSnapshot) plugin(name string) (pluginSnapshot, bool) {
	for _, plugin := range s.Plugins {
		if plugin.Name == name {
			return plugin, true
		}
	}
	return pluginSnapshot{}, false
}

func (s hostSnapshot) maxGeneration(name string) (int, bool) {
	plugin, ok := s.plugin(name)
	if !ok || len(plugin.Instances) == 0 {
		return 0, false
	}
	max := 0
	for _, instance := range plugin.Instances {
		if instance.Generation > max {
			max = instance.Generation
		}
	}
	return max, true
}

// availability is the runtime gate for list/call. Registration only requires a
// declared target capability; the target method, active instance and generation
// are checked here.
func (s hostSnapshot) availability(source, capability, method string, generation int) (bool, string) {
	plugin, ok := s.plugin(source)
	if !ok {
		return false, "source plugin is not loaded"
	}
	if !plugin.Enabled {
		return false, "source plugin is disabled"
	}
	if !containsString(plugin.Provides, capability) {
		return false, "source plugin does not provide " + capability
	}
	current, ok := s.maxGeneration(source)
	if !ok {
		return false, "source plugin has no instance"
	}
	if current != generation {
		return false, "source generation changed; resync required"
	}
	for _, instance := range plugin.Instances {
		if instance.State != "active" {
			continue
		}
		for _, registration := range instance.Registrations {
			if registration.Capability == capability && registration.Method == method {
				return true, ""
			}
		}
	}
	return false, "target method is not registered"
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// sourceGeneration returns the current generation for a provider. It accepts a
// loading instance: providers call tools.sync before host.register completes.
func (p *toolsPlugin) sourceGeneration(ctx context.Context, source string) (hostSnapshot, int, error) {
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return hostSnapshot{}, 0, pluginsdk.ErrCode("tool_source_invalid", "cannot verify source plugin: "+err.Error())
	}
	if _, ok := snapshot.plugin(source); !ok {
		return hostSnapshot{}, 0, pluginsdk.ErrCode("tool_source_invalid", "source plugin is not loaded")
	}
	generation, ok := snapshot.maxGeneration(source)
	if !ok {
		return snapshot, 0, pluginsdk.ErrCode("tool_source_invalid", "source plugin has no instance")
	}
	return snapshot, generation, nil
}
