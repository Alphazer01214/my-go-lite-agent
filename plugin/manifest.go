// Package plugin defines the on-disk Plugin manifest.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manifest is plugin.json next to a Plugin executable.
type Manifest struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Protocol int      `json:"protocol"`
	Provides []string `json:"provides"`
	Consumes []string `json:"consumes"`
	Entry    string   `json:"entry"`
}

// CurrentProtocol is the Frame/manifest protocol version this Host speaks.
const CurrentProtocol = 1

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
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if m.Protocol != CurrentProtocol {
		return fmt.Errorf("protocol must be %d, got %d", CurrentProtocol, m.Protocol)
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
