package webui

import (
	"sort"

	"github.com/tomori/my-go-lite-agent/internal/host"
)

// PluginStateProvider supplies live lifecycle state without exposing Host internals to Browser Call.
type PluginStateProvider interface {
	PluginSnapshot() []host.PluginSnapshot
}

func mergePluginSnapshots(discovered []BootstrapPlugin, runtime []host.PluginSnapshot) []BootstrapPlugin {
	items := make([]BootstrapPlugin, 0, len(discovered)+len(runtime))
	index := make(map[string]int, len(discovered)+len(runtime))
	for _, item := range discovered {
		if _, exists := index[item.Name]; exists {
			continue
		}
		index[item.Name] = len(items)
		items = append(items, item)
	}

	for _, state := range runtime {
		if i, exists := index[state.Name]; exists {
			items[i].PluginSnapshot = state
			continue
		}
		index[state.Name] = len(items)
		items = append(items, BootstrapPlugin{PluginSnapshot: state})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}
