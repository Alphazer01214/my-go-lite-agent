package plugin

import (
	"bytes"
	"encoding/json"
	"os"
)

type Manifest struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Protocol  int    `json:"protocol"`
	Autostart bool   `json:"autostart,omitempty"`
	// Entry: for example agent or agent.exe
	Entry     string `json:"entry"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`

	// Provides: the capability this plugin provides
	Provides []string `json:"provides,omitempty"`
	// Requires: the capability this plugin requires
	Requires []string `json:"requires,omitempty"`
	// DependsOn: the plugin names this plugin depends on
	DependsOn []string `json:"depends_on,omitempty"`
	// HostFaces: the host faces this plugin requires, for example commands/config
	HostFaces []string `json:"host_faces,omitempty"`

	Commands []Command `json:"commands,omitempty"`
	WebUI    *WebUI    `json:"ui,omitempty"`
}

// WebUI / layout
type WebUI struct {
	// Entry: for example main.js
	Entry string `json:"entry"`
	Trust string `json:"trust,omitempty"`

	Assets []string   `json:"assets,omitempty"`
	Pages  []WebPage  `json:"pages,omitempty"`
	Mounts []WebMount `json:"mounts,omitempty"`
}

type WebPage struct {
	Slug  string    `json:"slug"`
	Title string    `json:"title,omitempty"`
	Path  string    `json:"path"`
	Slots []WebSlot `json:"slots"`
}

type WebMount struct {
	Page      string          `json:"page,omitempty"`
	Slot      string          `json:"slot"`
	Component string          `json:"component"`
	Property  json.RawMessage `json:"property,omitempty"`
}

type WebSlot struct {
	ID        string `json:"id"`
	Role      string `json:"role,omitempty"`
	Preferred string `json:"preferred,omitempty"`
	Region    string `json:"region,omitempty"`
}

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Usage       string `json:"usage,omitempty"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var res Manifest
	if err := json.Unmarshal(data, &res); err != nil {
		return Manifest{}, err
	}
	return res, nil
}
