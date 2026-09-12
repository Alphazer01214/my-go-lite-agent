// Package plugin defines the on-disk Plugin manifest.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
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

// UISpec declares optional Web Medium Panel assets (ADR-0009).
type UISpec struct {
	Entry string   `json:"entry"`
	Slots []string `json:"slots,omitempty"`
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
	if strings.TrimSpace(m.Entry) == "" {
		return fmt.Errorf("entry is required")
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
