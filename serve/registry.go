package serve

import (
	"fmt"
	"os"
	"sort"

	"github.com/tomori/my-go-lite-agent/discovery"
)

// RegistrySnapshot is the Host provides registry as read by observability
// surfaces (Plugin Graph /api/plugins, /lp). It is declaration/liveness data
// only — Host routes by plugin name, not by capability (ADR-0030).
type RegistrySnapshot struct {
	// Provides is the unique-owner Capability registry (cap → plugin).
	Provides map[string]string
	// Faces maps plugin name → its declared hostFaces (config|commands|ui).
	Faces map[string][]string
	// Degraded lists plugins whose consumes are currently unmet (ADR-0022).
	Degraded []string
	// ReconcileGen counts registry re-evaluations since Start.
	ReconcileGen int
}

// Registry returns a consistent snapshot of the provides registry.
func Registry(s *Server) RegistrySnapshot {
	return s.registry()
}

func (s *Server) registry() RegistrySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := RegistrySnapshot{
		Provides:     make(map[string]string, len(s.provides)),
		Faces:        make(map[string][]string, len(s.plugins)),
		ReconcileGen: s.reconcileGen,
	}
	for k, v := range s.provides {
		out.Provides[k] = v
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

// registerProvides indexes capability owners for observability/degraded.
// Non-tools conflicts fail; tools stay multi-owner in the snapshot only
// (Host no longer merges or routes tools, ADR-0030).
func (s *Server) registerProvides(mounted []discovery.Found) error {
	for _, p := range mounted {
		if p.Manifest.Entry == "" {
			continue
		}
		for _, capName := range p.Manifest.Provides {
			if capName == "tools" {
				s.mu.Lock()
				if _, ok := s.provides[capName]; !ok {
					s.provides[capName] = p.Manifest.Name
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

// reconcileConsumes recomputes the provides registry and degraded set from
// the live mount set (ADR-0022). It is idempotent and full-graph.
func (s *Server) reconcileConsumes() {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]string, len(s.provides))
	for name, p := range s.plugins {
		if !p.healthy {
			continue
		}
		for _, capName := range p.found.Manifest.Provides {
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

	s.provides = next
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
