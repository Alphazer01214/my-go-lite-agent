// Package plugin defines the on-disk Plugin manifest.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// CommandSpec is one slash command a Plugin declares in its Manifest.
type CommandSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Usage       string `json:"usage"`
}

// UISpec declares optional Web Medium Panel components (ADR-0010).
type UISpec struct {
	// Entry is the plugin's UI Entry ES Module, relative to the plugin's ui/ dir.
	Entry  string    `json:"entry"`
	Mounts []UIMount `json:"mounts,omitempty"`
}

// UIMount statically mounts one Panel Component into a Panel slot at startup.
type UIMount struct {
	Slot      string          `json:"slot"`
	Component string          `json:"component"`
	Props     json.RawMessage `json:"props,omitempty"`
}

// UISlots are the fixed Panel slots the Shell provides (ADR-0009).
var UISlots = []string{"sidebar", "main-overlay", "toolbar-right"}

// ValidUISlot reports whether slot is a Shell Panel slot.
func ValidUISlot(slot string) bool {
	for _, s := range UISlots {
		if s == slot {
			return true
		}
	}
	return false
}

// NormalizedEntry returns the slash entry path guaranteed to resolve under ui/.
func (u UISpec) NormalizedEntry() string {
	clean := path.Clean(strings.ReplaceAll(u.Entry, "\\", "/"))
	if !strings.HasPrefix(clean, "ui/") {
		clean = "ui/" + clean
	}
	return clean
}

// Manifest is plugin.json next to a Plugin executable.
type Manifest struct {
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Protocol    int           `json:"protocol"`
	Provides    []string      `json:"provides"`
	Consumes    []string      `json:"consumes"`
	Entry       string        `json:"entry"`
	TimeoutMs   int           `json:"timeoutMs,omitempty"`
	Description string        `json:"description,omitempty"`
	Commands    []CommandSpec `json:"commands,omitempty"`
	UI          *UISpec       `json:"ui,omitempty"`
}

// CurrentProtocol is the Frame/manifest protocol version this Host speaks.
const CurrentProtocol = 2

var namePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// ReservedCommandNames are Host-native slash commands. Plugins whose name
// collides (case-insensitive) are rejected at Assembly (ADR-0008).
var ReservedCommandNames = map[string]bool{
	"help":    true,
	"lp":      true,
	"refresh": true,
	"exit":    true,
}

// ConflictsWithNativeCommand reports whether the Manifest name collides with a Host command.
func (m Manifest) ConflictsWithNativeCommand() bool {
	return ReservedCommandNames[strings.ToLower(m.Name)]
}

// LoadManifest reads and validates a plugin.json path.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest %s: %w", path, err)
	}
	return &m, nil
}

// Validate checks required fields. Dir is the Plugin directory used to resolve Entry.
func (m *Manifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("name %q must match [a-z0-9-]+", m.Name)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if m.Protocol < 1 || m.Protocol > CurrentProtocol {
		return fmt.Errorf("protocol must be 1..%d, got %d", CurrentProtocol, m.Protocol)
	}
	// UI-only Plugins are legal (ADR-0011): a Manifest must carry an executable
	// entry or a Web UI, never neither.
	if strings.TrimSpace(m.Entry) == "" && m.UI == nil {
		return fmt.Errorf("entry or ui is required")
	}
	for i, c := range m.Provides {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("provides[%d] is empty", i)
		}
	}
	for i, c := range m.Consumes {
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("consumes[%d] is empty", i)
		}
	}
	for i, c := range m.Commands {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("commands[%d].name is required", i)
		}
		if strings.ContainsAny(c.Name, " \t") {
			return fmt.Errorf("commands[%d].name %q must not contain whitespace", i, c.Name)
		}
	}
	if m.UI != nil {
		if err := m.UI.validate(m.Name); err != nil {
			return err
		}
	}
	return nil
}

var elementTagPattern = regexp.MustCompile(`^[a-z0-9-]*$`)

// ValidComponentTag reports whether tag is a legal custom element name: what
// customElements.define enforces (lowercase, starts with a letter, contains a
// hyphen). Shared by Manifest validation and serve's PanelOp validation.
func ValidComponentTag(tag string) bool {
	if len(tag) < 3 || tag[0] < 'a' || tag[0] > 'z' || !strings.Contains(tag, "-") {
		return false
	}
	return elementTagPattern.MatchString(tag)
}

// validate enforces the Panel Component contract (ADR-0010): the UI Entry is
// an ES Module under ui/, and every mounted component tag belongs to this
// plugin's namespace and targets a real Shell slot.
func (u *UISpec) validate(pluginName string) error {
	if strings.TrimSpace(u.Entry) == "" {
		return fmt.Errorf("ui.entry is required")
	}
	clean := path.Clean(strings.ReplaceAll(u.Entry, "\\", "/"))
	if strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
		return fmt.Errorf("ui.entry %q must stay under ui/", u.Entry)
	}
	if !strings.HasSuffix(clean, ".js") {
		return fmt.Errorf("ui.entry %q must be a .js ES Module", u.Entry)
	}
	for i, mount := range u.Mounts {
		if !ValidUISlot(mount.Slot) {
			return fmt.Errorf("ui.mounts[%d].slot %q must be one of %v", i, mount.Slot, UISlots)
		}
		if !ValidComponentTag(mount.Component) {
			return fmt.Errorf("ui.mounts[%d].component %q must be a valid custom element tag", i, mount.Component)
		}
		if !strings.HasPrefix(mount.Component, pluginName+"-") {
			return fmt.Errorf("ui.mounts[%d].component %q must be prefixed with %q", i, mount.Component, pluginName+"-")
		}
		if len(mount.Props) > 0 {
			trimmed := strings.TrimSpace(string(mount.Props))
			if !json.Valid(mount.Props) || !strings.HasPrefix(trimmed, "{") {
				return fmt.Errorf("ui.mounts[%d].props must be a JSON object", i)
			}
		}
	}
	return nil
}

// UIEntryExists reports whether the UI Entry module is present under dir.
func (m *Manifest) UIEntryExists(dir string) error {
	full := filepath.Join(dir, filepath.FromSlash(m.UI.NormalizedEntry()))
	st, err := os.Stat(full)
	if err != nil {
		return fmt.Errorf("ui.entry %q not found in %s: %w", m.UI.Entry, dir, err)
	}
	if st.IsDir() {
		return fmt.Errorf("ui.entry %q is a directory in %s", m.UI.Entry, dir)
	}
	return nil
}

// ResolveEntry returns the absolute path of the executable under dir.
func (m *Manifest) ResolveEntry(dir string) string {
	return filepath.Join(dir, m.Entry)
}

// EntryExists reports whether the executable file is present under dir.
func (m *Manifest) EntryExists(dir string) error {
	p := m.ResolveEntry(dir)
	st, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("entry %q not found in %s: %w", m.Entry, dir, err)
	}
	if st.IsDir() {
		return fmt.Errorf("entry %q is a directory in %s", m.Entry, dir)
	}
	return nil
}
