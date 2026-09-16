package serve

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// RegistrySnapshot is the Host Capability registry as read by observability
// surfaces (Plugin Graph /api/plugins, /lp). It shows what actually routes
// today, not what manifests declare.
type RegistrySnapshot struct {
	// Provides is the unique-owner Capability registry (cap → plugin).
	Provides map[string]string
	// ToolOwners is the tool-name → providing-plugin map.
	ToolOwners map[string]string
	// Faces maps plugin name → its declared hostFaces (config|commands|ui).
	Faces map[string][]string
	// Degraded lists plugins whose consumes are currently unmet (ADR-0022).
	Degraded []string
	// ReconcileGen counts registry re-evaluations since Start.
	ReconcileGen int
}

// Registry returns a consistent snapshot of the Capability registry.
// Package function so observability surfaces (web) read it without expanding
// the Server's exported method surface (ADR-0016 allowlist).
func Registry(s *Server) RegistrySnapshot {
	return s.registry()
}

func (s *Server) registry() RegistrySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := RegistrySnapshot{
		Provides:     make(map[string]string, len(s.provides)),
		ToolOwners:   make(map[string]string, len(s.toolOwners)),
		Faces:        make(map[string][]string, len(s.plugins)),
		ReconcileGen: s.reconcileGen,
	}
	for k, v := range s.provides {
		out.Provides[k] = v
	}
	for k, v := range s.toolOwners {
		out.ToolOwners[k] = v
	}
	for name, p := range s.plugins {
		if f := p.found.Manifest.HostFaces; len(f) > 0 {
			out.Faces[name] = append([]string(nil), f...)
		}
	}
	for n := range s.degraded {
		out.Degraded = append(out.Degraded, n)
	}
	sort.Strings(out.Degraded)
	return out
}

// registerProvides indexes capability owners. Non-tools conflicts fail; tools are multi-owner (ADR-0018).
func (s *Server) registerProvides(mounted []discovery.Found) error {
	for _, p := range mounted {
		if p.Manifest.Entry == "" {
			continue
		}
		for _, capName := range p.Manifest.Provides {
			if capName == ToolsCap {
				s.mu.Lock()
				dup := false
				for _, existing := range s.toolsProviders {
					if existing == p.Manifest.Name {
						dup = true
						break
					}
				}
				if !dup {
					s.toolsProviders = append(s.toolsProviders, p.Manifest.Name)
				}
				if _, ok := s.provides[ToolsCap]; !ok {
					s.provides[ToolsCap] = p.Manifest.Name
				}
				s.mu.Unlock()
				continue
			}
			s.mu.Lock()
			if owner, ok := s.provides[capName]; ok && owner != p.Manifest.Name {
				s.mu.Unlock()
				return fmt.Errorf("capability %q provided by both %s and %s", capName, owner, p.Manifest.Name)
			}
			s.provides[capName] = p.Manifest.Name
			s.mu.Unlock()
		}
	}
	return nil
}

// discoverTools asks every tools Provider for its schema list and builds the
// tool-name → plugin map (ADR-0018). The new map is built locally and swapped
// only on full success: any provider failure keeps the previous map. Duplicate
// tool names fail (same policy as toolsListMerged).
func (s *Server) discoverTools() error {
	s.mu.Lock()
	providers := append([]string(nil), s.toolsProviders...)
	s.mu.Unlock()
	next := make(map[string]string)
	for _, name := range providers {
		payload, err := s.callByPlugin(name, ToolsCap, "list", json.RawMessage(`{}`))
		if err != nil {
			return fmt.Errorf("tools.list from %s: %w", name, err)
		}
		var out struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &out)
		}
		for _, t := range out.Tools {
			if t.Name == "" {
				continue
			}
			if prev, ok := next[t.Name]; ok && prev != name {
				return fmt.Errorf("tool %q provided by both %s and %s", t.Name, prev, name)
			}
			next[t.Name] = name
		}
	}
	s.mu.Lock()
	s.toolOwners = next
	s.mu.Unlock()
	return nil
}

// toolsListMerged fans out tools.list and merges schemas (ADR-0018).
func (s *Server) toolsListMerged() (json.RawMessage, error) {
	s.mu.Lock()
	providers := append([]string(nil), s.toolsProviders...)
	s.mu.Unlock()
	if len(providers) == 0 {
		return nil, &protocol.FrameError{Code: "capability_unavailable", Message: "no plugin provides tools"}
	}
	merged := []any{}
	seen := map[string]bool{}
	for _, name := range providers {
		payload, err := s.callByPlugin(name, ToolsCap, "list", json.RawMessage(`{}`))
		if err != nil {
			return nil, err
		}
		var out struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &out)
		}
		for _, t := range out.Tools {
			var meta struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(t, &meta)
			if meta.Name == "" {
				continue
			}
			// Same duplicate policy as discoverTools: fail loud, no silent first-wins.
			if seen[meta.Name] {
				return nil, fmt.Errorf("tool %q provided by multiple plugins", meta.Name)
			}
			seen[meta.Name] = true
			merged = append(merged, json.RawMessage(t))
		}
	}
	return MarshalPayload(map[string]any{"tools": merged}), nil
}

// toolsOwnerFor routes a tools.call by tool name (ADR-0018).
func (s *Server) toolsOwnerFor(toolName string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner, ok := s.toolOwners[toolName]
	return owner, ok
}

// reconcileConsumes recomputes the Capability registry and degraded set from
// the live mount set (ADR-0022). It is idempotent and full-graph: provides can
// regress (a provider becomes degraded) or return (a dependency is mounted),
// and degraded can clear. The next state is built locally, then swapped
// atomically; reconcileGen increments for observability (/api/plugins).
func (s *Server) reconcileConsumes() {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]string, len(s.provides))
	var nextTools []string
	for name, p := range s.plugins {
		if !p.healthy {
			continue
		}
		for _, capName := range p.found.Manifest.Provides {
			if capName == ToolsCap {
				if _, ok := next[ToolsCap]; !ok {
					next[ToolsCap] = name
				}
				if !containsString(nextTools, name) {
					nextTools = append(nextTools, name)
				}
				continue
			}
			if _, ok := next[capName]; !ok {
				next[capName] = name
			}
		}
	}

	degraded := make(map[string]bool, len(s.degraded))
	// Iterate to a fixpoint: a plugin whose consumes are all provided stays;
	// otherwise it is degraded and its provides are withdrawn, which may
	// degrade transitive consumers.
	for {
		changed := false
		for name, p := range s.plugins {
			if !p.healthy {
				continue
			}
			unmet := false
			for _, need := range p.found.Manifest.Consumes {
				if _, ok := next[need]; !ok {
					fmt.Fprintf(os.Stderr, "warn: plugin %s consumes %q but no mounted plugin provides it (degraded)\n", name, need)
					unmet = true
					break
				}
			}
			if unmet {
				if !degraded[name] {
					degraded[name] = true
					changed = true
				}
				for _, capName := range p.found.Manifest.Provides {
					if owner, ok := next[capName]; ok && owner == name {
						delete(next, capName)
					}
				}
			} else if degraded[name] {
				delete(degraded, name)
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Swap atomically: provides may regress, degraded may clear (ADR-0022).
	s.provides = next
	s.toolsProviders = nextTools
	for name := range s.degraded {
		if !degraded[name] {
			delete(s.degraded, name)
		}
	}
	for name := range degraded {
		s.degraded[name] = true
	}
	s.reconcileGen++
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
