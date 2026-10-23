package host

import (
	"sort"
)

// PluginSnapshot is one plugin-name group with all current Provider instances.
// State and SelectedInstanceID describe the deterministic default display.
type PluginSnapshot struct {
	Name               string                   `json:"name"`
	Version            string                   `json:"version,omitempty"`
	Description        string                   `json:"description,omitempty"`
	State              string                   `json:"state"`
	SelectedInstanceID string                   `json:"selected_instance_id,omitempty"`
	Instances          []PluginInstanceSnapshot `json:"instances,omitempty"`
	Enabled            bool                     `json:"enabled"`
	Missing            bool                     `json:"missing,omitempty"`
	Provides           []string                 `json:"provides,omitempty"`
	DependsOn          []string                 `json:"depends_on,omitempty"`
}

func selectProviderInstance(items []PluginInstanceSnapshot) (string, string) {
	if len(items) == 0 {
		return "", ProviderStateInactive
	}
	priority := map[string]int{
		ProviderStateActive:   0,
		ProviderStateFailed:   1,
		ProviderStateLoading:  2,
		ProviderStateStopping: 3,
		ProviderStateInactive: 4,
	}
	best := items[0]
	for _, item := range items[1:] {
		if priority[item.State] < priority[best.State] {
			best = item
			continue
		}
		if priority[item.State] == priority[best.State] && item.InstanceID < best.InstanceID {
			best = item
		}
	}
	return best.InstanceID, best.State
}

// PluginSnapshot returns deterministic plugin-name groups for host.plugins and WebUI.
func (h *Host) PluginSnapshot() []PluginSnapshot {
	h.mu.RLock()
	discoveries := h.discoveries
	instances := make(map[string][]PluginInstanceSnapshot, len(h.plugins))
	for _, p := range h.plugins {
		if p == nil || p.pluginName == "" {
			continue
		}
		instances[p.pluginName] = append(instances[p.pluginName], instanceSnapshot(p))
	}
	disabled := make(map[string]bool, len(h.disabled))
	for name, value := range h.disabled {
		disabled[name] = value
	}
	h.mu.RUnlock()

	for _, items := range instances {
		sort.Slice(items, func(i, j int) bool { return items[i].InstanceID < items[j].InstanceID })
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

		providerInstances := instances[manifest.Name]
		selectedID, state := selectProviderInstance(providerInstances)
		items = append(items, PluginSnapshot{
			Name:               manifest.Name,
			Version:            manifest.Version,
			Description:        manifest.Description,
			State:              state,
			SelectedInstanceID: selectedID,
			Instances:          append([]PluginInstanceSnapshot(nil), providerInstances...),
			Enabled:            !disabled[manifest.Name],
			Provides:           append([]string(nil), manifest.Provides...),
			DependsOn:          append([]string(nil), manifest.DependsOn...),
		})
	}

	for name := range referenced {
		if _, exists := seen[name]; exists {
			continue
		}
		items = append(items, PluginSnapshot{
			Name:    name,
			State:   ProviderStateInactive,
			Enabled: true,
			Missing: true,
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}
