package webui

import (
	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

var slots = []string{"top", "left", "center", "right", "bottom"}

type MountKey struct {
	Plugin string
	ID     string
}

type MountSpec struct {
	Key      MountKey
	Tag      string
	Slot     string
	EntryURL string
}

type MountRef struct {
	Plugin string `json:"plugin"`
	ID     string `json:"id"`
}

type BootstrapMount struct {
	Mount MountRef `json:"mount"`
	Tag   string   `json:"tag"`
	Slot  string   `json:"slot"`
	Entry string   `json:"entry"`
}

type BootstrapPlugin struct {
	host.PluginSnapshot
	Settings *plugin.SettingsManifest `json:"settings,omitempty"`
	Mounts   []MountRef               `json:"mounts,omitempty"`
}

type Bootstrap struct {
	Version     int                 `json:"version"`
	Slots       []string            `json:"slots"`
	Plugins     []BootstrapPlugin   `json:"plugins"`
	Mounts      []BootstrapMount    `json:"mounts"`
	Diagnostics []plugin.Diagnostic `json:"diagnostics"`
}
