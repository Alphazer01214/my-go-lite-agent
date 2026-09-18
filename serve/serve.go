// Package serve keeps mounted Plugins alive and forwards Frames by plugin name (ADR-0030).
package serve

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// DefaultCallTimeout is used when a Plugin manifest omits timeoutMs.
const DefaultCallTimeout = 30 * time.Second

// DefaultShutdownGrace is how long Close waits after stdin EOF before killing.
const DefaultShutdownGrace = 2 * time.Second

// Server owns Plugin processes and the lifecycle registry.
type Server struct {
	mu       sync.Mutex
	plugins  map[string]*proc
	provides map[string]string
	// catalog is the Discovery result (all known plugins) for ensurePlugins (ADR-0023).
	catalog discovery.Result
	// pluginsDir lets ensurePlugins re-scan when the catalog is stale (new binaries on disk).
	pluginsDir string
	// mountedUI tracks UI-only plugins already accepted into the plan.
	mountedUI map[string]bool
	// degraded marks plugins whose consumes are unmet (ADR-0022): not in provides registry.
	degraded map[string]bool
	// reconcileGen counts full registry re-evaluations (observability, /api/plugins).
	reconcileGen int
	pending      map[string]*wait
	closed       bool
	seq          int
	gen          map[string]int
	cards        []PresentationCard
	panels       []PanelOp
	subs         []*Subscriber
	job          *jobHolder
	// approvals are the registered Render Medium faces for policy.ask
	// (agent.confirm, ADR-0029). All registered faces are asked in parallel;
	// the first responder wins the ruling. No faces deny (safe default).
	approvals []func(tool string, arguments json.RawMessage, workspace, sessionID string) bool
	// disabled is the user plugin-switch denylist (ADR-0032, L0 by name).
	disabled map[string]bool
	// switchPath is where the denylist persists (usually <pluginsDir>/.plugin-switch.json).
	switchPath string
}

// HostCap is the Capability for Host cross-cutting methods (ensurePlugins).
const HostCap = "host"

// SetCatalog stores the Discovery result so ensurePlugins can mount later (ADR-0023).
func (s *Server) SetCatalog(res discovery.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.catalog = res
}

// SetPluginsDir records the Discovery root so EnsurePlugins can re-scan a stale catalog.
// Also loads the plugin-switch store from that root (ADR-0032).
func (s *Server) SetPluginsDir(dir string) {
	s.mu.Lock()
	s.pluginsDir = dir
	s.switchPath = SwitchPath(dir)
	s.mu.Unlock()
	s.LoadPluginSwitch()
}

// MountedPluginNames lists process/UI plugins currently mounted (for /lp).
func (s *Server) MountedPluginNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.plugins)+len(s.mountedUI))
	for n := range s.plugins {
		names = append(names, n)
	}
	for n := range s.mountedUI {
		if _, ok := s.plugins[n]; !ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// DegradedNames returns plugins marked degraded (consumes unmet, ADR-0022).
func (s *Server) DegradedNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for n := range s.degraded {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Start launches every mounted Plugin, reconciles consumes, and builds the
// provides registry (observability / degraded only — routing is by plugin name).
// Soft-fail (ADR-0017): a single Plugin launch failure never takes down the
// whole Host — the failure is surfaced on stderr and the remaining Plugins
// still start. Capability conflicts are likewise visible but non-fatal.
func Start(mounted []discovery.Found) (*Server, error) {
	s := &Server{
		plugins:   make(map[string]*proc, len(mounted)),
		provides:  make(map[string]string),
		pending:   make(map[string]*wait),
		gen:       make(map[string]int),
		mountedUI: make(map[string]bool),
		degraded:  make(map[string]bool),
		disabled:  make(map[string]bool),
	}
	job, err := newJob()
	if err != nil {
		// Non-fatal: fall back to explicit Close/killTree only.
		fmt.Fprintf(os.Stderr, "warn: job object unavailable: %v\n", err)
	} else {
		s.job = job
	}
	// Boot filter: honor any already-persisted plugin switch (ADR-0032).
	// Callers may SetPluginsDir first; path may still be empty → no-op load.
	s.LoadPluginSwitch()
	mounted = FilterMountedFound(mounted, s.DisabledPluginNamesSet())
	if err := s.registerProvides(mounted); err != nil {
		// Configuration error (duplicate capability owner): visible, continue —
		// the registry will simply not route the conflicting capability.
		fmt.Fprintf(os.Stderr, "warn: %v\n", err)
	}
	for _, p := range mounted {
		// UI-only Plugin: no executable, no process, no Frames (ADR-0011).
		if p.Manifest.Entry == "" {
			s.mu.Lock()
			s.mountedUI[p.Manifest.Name] = true
			s.mu.Unlock()
			continue
		}
		if err := s.launch(p); err != nil {
			// ADR-0017: one bad Plugin must not take down the Host.
			fmt.Fprintf(os.Stderr, "warn: launch failed plugin=%s err=%v\n", p.Manifest.Name, err)
			continue
		}
	}
	s.reconcileConsumes()
	return s, nil
}

// DisabledPluginNamesSet returns the denylist as a map (boot filter helper).
func (s *Server) DisabledPluginNamesSet() map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]bool, len(s.disabled))
	for k, v := range s.disabled {
		if v {
			out[k] = true
		}
	}
	return out
}

// CallByFace routes a Host-addressed face call into a Plugin that declared the
// hostFace (config | commands | ui, ADR-0027). The face must be declared or
// the call is rejected — this is what makes bypassing the registry impossible
// at the call-site level.
func CallByFace(s *Server, pluginName, face, method string, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	s.mu.Lock()
	p := s.plugins[pluginName]
	s.mu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("plugin %q not mounted", pluginName)
	}
	declared := false
	for _, f := range p.found.Manifest.HostFaces {
		if f == face {
			declared = true
			break
		}
	}
	if !declared {
		return nil, fmt.Errorf("plugin %q does not declare hostFace %q", pluginName, face)
	}
	out, err := s.call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     face,
		Method:  method,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Payload, nil
}

// CallByPlugin addresses a mounted Plugin by name and invokes cap.method
// on it (L0 point-named call, ADR-0030). Cap/Method are the receiving
// Plugin's dispatch keys; Host does not route by them.
func (s *Server) CallByPlugin(pluginName, cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	return s.callByPlugin(pluginName, cap, method, payload)
}

// MarshalPayload JSON-encodes v for a Call payload.
func MarshalPayload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
