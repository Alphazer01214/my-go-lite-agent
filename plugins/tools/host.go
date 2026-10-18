package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type instanceSnapshot struct {
	State         string                 `json:"state"`
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

func (s hostSnapshot) available(source, capability, method string) bool {
	for _, plugin := range s.Plugins {
		if plugin.Name != source || !plugin.Enabled {
			continue
		}
		if plugin.State != "active" {
			return false
		}
		for _, provided := range plugin.Provides {
			if provided == capability {
				for _, instance := range plugin.Instances {
					if instance.State != "active" {
						continue
					}
					for _, registration := range instance.Registrations {
						if registration.Capability == capability && registration.Method == method {
							return true
						}
					}
				}
				return false
			}
		}
		return false
	}
	return false
}

func (p *toolsPlugin) requireSource(ctx context.Context, source, capability, method string) error {
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return pluginsdk.ErrCode("tool_source_invalid", "cannot verify source plugin: "+err.Error())
	}
	if !snapshot.available(source, capability, method) {
		return pluginsdk.ErrCode("tool_source_invalid",
			fmt.Sprintf("%s is not active or has not registered %s.%s", source, capability, method))
	}
	return nil
}
