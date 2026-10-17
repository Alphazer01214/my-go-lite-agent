package host

import (
	"sort"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

type pluginProcessState struct {
	healthy bool
}

// PluginSnapshot is the management-plane view of one discovered or referenced plugin.
// It intentionally contains only lifecycle and static manifest metadata, never local paths.
type PluginSnapshot struct {
	Name      string   `json:"name"`
	Version   string   `json:"version,omitempty"`
	State     string   `json:"state"`
	Enabled   bool     `json:"enabled"`
	Provides  []string `json:"provides,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// PluginSnapshot returns deterministic plugin lifecycle state for host.plugins and WebUI Bootstrap.
func (h *Host) PluginSnapshot() []PluginSnapshot {
	h.mu.RLock()
	discoveries := h.discoveries
	processes := make(map[string]pluginProcessState, len(h.plugins))
	for instanceID, proc := range h.plugins {
		name := instanceID
		if proc != nil && proc.pluginName != "" {
			name = proc.pluginName
		}
		state := processes[name]
		if proc != nil && proc.healthy {
			state.healthy = true
		}
		processes[name] = state
	}
	disabled := make(map[string]bool, len(h.disabled))
	stopped := make(map[string]bool, len(h.stopped))
	startupPlanned := make(map[string]bool, len(h.startupPlanned))
	for name, value := range h.disabled {
		disabled[name] = value
	}
	for name, value := range h.stopped {
		stopped[name] = value
	}
	for name, value := range h.startupPlanned {
		startupPlanned[name] = value
	}
	h.mu.RUnlock()

	ordered, _ := plugin.AutostartClosure(discoveries)
	for _, discovery := range ordered {
		startupPlanned[discovery.Manifest.Name] = true
	}

	seen := make(map[string]struct{}, len(discoveries.Plugins))
	referenced := make(map[string]struct{})
	items := make([]PluginSnapshot, 0, len(discoveries.Plugins)*2)
	for _, discovery := range discoveries.Plugins {
		manifest := discovery.Manifest
		if manifest.Name == "" {
			continue
		}
		if _, exists := seen[manifest.Name]; exists {
			continue
		}
		seen[manifest.Name] = struct{}{}

		for _, dependency := range manifest.DependsOn {
			referenced[dependency] = struct{}{}
		}

		state := PluginStateAvailable
		enabled := !disabled[manifest.Name]
		_, processExists := processes[manifest.Name]
		plannedToStart := startupPlanned[manifest.Name]
		switch {
		case !enabled:
			state = PluginStateDisabled
		case processes[manifest.Name].healthy:
			state = PluginStateMounted
		case stopped[manifest.Name]:
			state = PluginStateAvailable
		case processExists || plannedToStart:
			state = PluginStateDegraded
		}

		items = append(items, PluginSnapshot{
			Name:      manifest.Name,
			Version:   manifest.Version,
			State:     state,
			Enabled:   enabled,
			Provides:  append([]string(nil), manifest.Provides...),
			DependsOn: append([]string(nil), manifest.DependsOn...),
		})
	}

	for name := range referenced {
		if _, exists := seen[name]; exists {
			continue
		}
		items = append(items, PluginSnapshot{
			Name:    name,
			State:   PluginStateMissing,
			Enabled: true,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}
