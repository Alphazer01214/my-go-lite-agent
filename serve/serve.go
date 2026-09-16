// Package serve keeps mounted Plugins alive and routes Capabilities star-through Host.
package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// DefaultCallTimeout is used when a Plugin manifest omits timeoutMs.
const DefaultCallTimeout = 30 * time.Second

// DefaultShutdownGrace is how long Close waits after stdin EOF before killing.
const DefaultShutdownGrace = 2 * time.Second

type waitKind int

const (
	waitHost waitKind = iota
	waitPlugin
)

type wait struct {
	kind       waitKind
	caller     string
	target     string
	origID     string
	cap        string
	method     string
	reqPayload json.RawMessage
	ch         chan *CallResult
	events     []*protocol.Frame
	onEvent    func(*protocol.Frame)
}

// CallResult carries one Call's final res plus any evt frames collected while waiting.
type CallResult struct {
	Frame  *protocol.Frame
	Events []*protocol.Frame
}

type proc struct {
	found   discovery.Found
	cmd     *exec.Cmd
	stdin   *os.File
	wmu     sync.Mutex
	healthy bool
	timeout time.Duration
	gen     int
}

// Server owns Plugin processes and the Capability registry.
type Server struct {
	mu       sync.Mutex
	plugins  map[string]*proc
	provides map[string]string
	// toolsProviders lists Plugins that provide the multi-owner tools Capability (ADR-0018).
	toolsProviders []string
	// toolOwners maps a tool name to the Plugin that registered it via tools.list.
	toolOwners map[string]string
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
	pending  map[string]*wait
	closed   bool
	seq      int
	gen      map[string]int
	cards    []PresentationCard
	panels   []PanelOp
	subs     []*Subscriber
	// turnStates serializes turns per Session id (parallel across sessions).
	turnStatesMu sync.Mutex
	turnStates   map[string]*sessionTurn
	job          *jobHolder
	// OnStreamDelta is the live Render Medium hook for ephemeral stream chunks.
	// channel is "content" or "reasoning" so media can paint them differently.
	OnStreamDelta func(delta, channel string)
	// OnStatus is the live Render Medium hook for agent idle/running.
	OnStatus func(status string)
	// OnToolCall is the live Render Medium hook when the Loop starts a tool (or Subagent).
	OnToolCall func(name string, arguments json.RawMessage)
	// OnRender is the live Render Medium hook for presentation.render intents.
	OnRender func(ri RenderIntent)
	// OnToolApproval is the live Render Medium hook for policy.ask (agent.confirm).
	// Return true to allow the tool call. Nil means deny (safe default).
	OnToolApproval func(tool string, arguments json.RawMessage, workspace, sessionID string) bool
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
func (s *Server) SetPluginsDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pluginsDir = dir
}

// ensureCatalog returns a fresh Discovery result when pluginsDir is known.
func (s *Server) ensureCatalog() discovery.Result {
	s.mu.Lock()
	dir := s.pluginsDir
	cached := s.catalog
	s.mu.Unlock()
	if dir != "" {
		if res := discovery.Scan(dir); len(res.Plugins) > 0 {
			s.mu.Lock()
			s.catalog = res
			s.mu.Unlock()
			return res
		}
	}
	return cached
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

// ToolOwners returns a copy of the tool-name → providing-plugin map (ADR-0018),
// for observability surfaces such as the Web plugin graph.
func (s *Server) ToolOwners() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.toolOwners))
	for k, v := range s.toolOwners {
		out[k] = v
	}
	return out
}

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
func (s *Server) Registry() RegistrySnapshot {
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

// PanelOp is one Web Medium panel mutation (ADR-0010): mount (set) or remove
// (clear) a Panel Component by id. See CONTEXT.md PanelOp.
type PanelOp struct {
	Op        string          `json:"op"`                  // set | clear
	Slot      string          `json:"slot"`                // sidebar | main-overlay | toolbar-right
	ID        string          `json:"id"`                  // stable panel id; set replaces by id
	Component string          `json:"component,omitempty"` // custom element tag, required for set
	Props     json.RawMessage `json:"props,omitempty"`     // JSON object passed to the element
}

// PresentationPanelMethod is a PanelOp op (set|clear). See CONTEXT.md PanelOp.
const PresentationPanelMethod = "panel"

// UICap is the Capability for UI Action routing from the Web Shell.
const UICap = "ui"

// UIActionMethod is the method Plugins implement to handle UI Actions.
const UIActionMethod = "action"

// PresentationCap is the Capability used for Presentation Card evt frames.
const PresentationCap = "presentation"

// PresentationCardMethod is the evt method for a Presentation Card.
const PresentationCardMethod = "card"

// PresentationStreamMethod is the ephemeral stream evt (start/chunk/end); not logged.
const PresentationStreamMethod = "stream"

// PresentationStatusMethod is the agent status evt (idle/running) for Render Medium.
const PresentationStatusMethod = "status"

// PresentationRenderMethod is a classified render intent (markdown_text|message_text|summary_text).
const PresentationRenderMethod = "render"

// CommandsCap is the Capability Host uses to route slash commands into a Plugin.
const CommandsCap = "commands"

// CommandsCallMethod is the method Plugins implement to handle a slash command.
const CommandsCallMethod = "call"

// PresentationCard is a structured UI render intent projected from args/result (no I/O).
type PresentationCard struct {
	CardType string          `json:"cardType"`
	Tool     string          `json:"tool,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// Cards returns a copy of Presentation Cards observed this run.
func (s *Server) Cards() []PresentationCard {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PresentationCard, len(s.cards))
	copy(out, s.cards)
	return out
}

func (s *Server) recordCard(f *protocol.Frame) {
	var c PresentationCard
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &c); err != nil {
			return
		}
	}
	if c.CardType == "" {
		return
	}
	s.mu.Lock()
	s.cards = append(s.cards, c)
	s.mu.Unlock()
}

// Start launches every mounted Plugin, soft-checks consumes, and builds the Capability registry.
func Start(mounted []discovery.Found) (*Server, error) {
	s := &Server{
		plugins:    make(map[string]*proc, len(mounted)),
		provides:   make(map[string]string),
		toolOwners: make(map[string]string),
		pending:    make(map[string]*wait),
		gen:        make(map[string]int),
		mountedUI:  make(map[string]bool),
		degraded:   make(map[string]bool),
	}
	job, err := newJob()
	if err != nil {
		// Non-fatal: fall back to explicit Close/killTree only.
		fmt.Fprintf(os.Stderr, "warn: job object unavailable: %v\n", err)
	} else {
		s.job = job
	}
	if err := s.registerProvides(mounted); err != nil {
		_ = s.Close()
		return nil, err
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
			debugf("launch failed plugin=%s err=%v", p.Manifest.Name, err)
			_ = s.Close()
			return nil, err
		}
	}
	s.reconcileConsumes()
	if err := s.discoverTools(); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
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
		payload, err := s.CallByPlugin(name, ToolsCap, "list", json.RawMessage(`{}`))
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

// CallByPlugin is Host-initiated cap.method to a named Plugin (bypasses unique-owner lookup).
func (s *Server) CallByPlugin(pluginName, cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	out, err := s.Call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     cap,
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
		payload, err := s.CallByPlugin(name, ToolsCap, "list", json.RawMessage(`{}`))
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

func (s *Server) launch(found discovery.Found) error {
	name := found.Manifest.Name
	entry := found.Manifest.ResolveEntry(found.Dir)
	cmd := exec.Command(entry)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin %s: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("stdout %s: %w", name, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}
	// Bind child to Host lifetime (Windows Job Object: kill-on-parent-close).
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()
	if err := job.assign(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "warn: assign %s to job: %v\n", name, err)
	}
	to := DefaultCallTimeout
	if found.Manifest.TimeoutMs > 0 {
		to = time.Duration(found.Manifest.TimeoutMs) * time.Millisecond
	}
	debugf("launch start plugin=%s", name)
	s.mu.Lock()
	s.gen[name]++
	g := s.gen[name]
	if old, ok := s.plugins[name]; ok && old != nil && old.stdin != nil {
		old.wmu.Lock()
		_ = old.stdin.Close()
		old.wmu.Unlock()
	}
	s.plugins[name] = &proc{
		found:   found,
		cmd:     cmd,
		stdin:   stdin.(*os.File),
		healthy: true,
		timeout: to,
		gen:     g,
	}
	s.mu.Unlock()
	debugf("launch plugin=%s gen=%d entry=%s", name, g, entry)
	go s.readLoop(name, g, stdout)
	return nil
}

func (s *Server) readLoop(pluginName string, gen int, stdout io.Reader) {
	for {
		f, err := protocol.ReadFrame(stdout)
		if err != nil {
			debugf("<- plugin=%s read_loop_exit err=%v", pluginName, err)
			s.markUnhealthy(pluginName, gen)
			return
		}
		debugFrame("<-", pluginName, f)
		s.handleFromPlugin(pluginName, f)
	}
}

func (s *Server) markUnhealthy(pluginName string, gen int) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	p := s.plugins[pluginName]
	if p == nil || p.gen != gen {
		s.mu.Unlock()
		return
	}
	p.healthy = false
	debugf("unhealthy plugin=%s gen=%d", pluginName, gen)
	var hostFail []*wait
	var pluginFail []struct{ caller, origID string }
	for id, w := range s.pending {
		if w.target == pluginName {
			delete(s.pending, id)
			if w.kind == waitHost {
				hostFail = append(hostFail, w)
			} else {
				pluginFail = append(pluginFail, struct{ caller, origID string }{w.caller, w.origID})
			}
		}
	}
	s.mu.Unlock()

	for _, w := range hostFail {
		select {
		case w.ch <- &CallResult{Frame: &protocol.Frame{
			Type:  protocol.TypeRes,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		}}:
		default:
		}
	}
	for _, pf := range pluginFail {
		_ = s.writeTo(pf.caller, &protocol.Frame{
			Type:  protocol.TypeRes,
			ID:    pf.origID,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		})
	}
	// A dead plugin can no longer serve its provides: re-register the registry
	// so its consumers degrade and routing fails honestly (ADR-0022).
	s.reconcileConsumes()
}

func (s *Server) ensureAlive(name string) error {
	s.mu.Lock()
	p := s.plugins[name]
	if p == nil {
		s.mu.Unlock()
		return fmt.Errorf("plugin %s not mounted", name)
	}
	if p.healthy {
		s.mu.Unlock()
		return nil
	}
	found := p.found
	old := p.cmd
	s.mu.Unlock()

	// Reap old process without hanging the caller. Do not Wait here:
	// Close owns Wait; a second Wait on the same Cmd panics.
	if old != nil {
		killTree(old)
	}
	s.mu.Lock()
	// Another goroutine may have restarted already.
	if cur := s.plugins[name]; cur != nil && cur.healthy {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if err := s.launch(found); err != nil {
		return err
	}
	// The revived plugin may now satisfy consumes and its tools may differ
	// after the crash-restart: reconcile the registry and re-discover tools.
	go func() {
		s.reconcileConsumes()
		_ = s.discoverTools()
	}()
	return nil
}

func (s *Server) handleFromPlugin(from string, f *protocol.Frame) {
	switch f.Type {
	case protocol.TypeReq:
		s.routeRequest(from, f)
	case protocol.TypeRes:
		s.complete(f)
	case protocol.TypeEvt:
		s.collectEvent(from, f)
	}
}

func (s *Server) collectEvent(from string, f *protocol.Frame) {
	// Presentation Cards are broadcast (may have no id); record before id filter.
	if f.Cap == PresentationCap && f.Method == PresentationCardMethod {
		s.recordCard(f)
	}
	if f.Cap == PresentationCap && f.Method == PresentationRenderMethod {
		s.dispatchRender(f)
	}
	if f.Cap == PresentationCap && f.Method == PresentationPanelMethod {
		s.dispatchPanel(from, f)
	}
	// Live stream chunks are a Presentation signal (ADR-0016): fan out even when
	// the LLM Plugin was called by an external Agent via the star, not by Host.
	if delta, channel, sessionID, ok := extractStreamDelta(f); ok {
		if s.OnStreamDelta != nil {
			s.OnStreamDelta(delta, channel)
		}
		s.publish(Event{Topic: "stream", Data: map[string]string{
			"delta":     delta,
			"channel":   channel,
			"sessionId": sessionID,
		}})
	}
	if f.ID == "" {
		return
	}
	s.mu.Lock()
	var cb func(*protocol.Frame)
	if w, ok := s.pending[f.ID]; ok {
		w.events = append(w.events, f)
		cb = w.onEvent
	}
	s.mu.Unlock()
	if cb != nil {
		cb(f)
	}
}

func (s *Server) dispatchRender(f *protocol.Frame) {
	var ri struct {
		Kind      string        `json:"kind"`
		Text      string        `json:"text"`
		Level     string        `json:"level"`
		Title     string        `json:"title"`
		Pairs     []SummaryPair `json:"pairs"`
		Detail    string        `json:"detail"`
		SessionID string        `json:"sessionId"`
	}
	if len(f.Payload) > 0 {
		_ = json.Unmarshal(f.Payload, &ri)
	}
	intent := RenderIntent{
		Kind:      ri.Kind,
		Text:      ri.Text,
		Level:     ri.Level,
		Title:     ri.Title,
		Pairs:     ri.Pairs,
		Detail:    ri.Detail,
		SessionID: ri.SessionID,
	}
	switch ri.Kind {
	case KindMarkdownText, KindMessageText, KindSummaryText:
		if s.OnRender != nil {
			s.OnRender(intent)
		}
		s.publish(Event{Topic: "presentation", Data: intent})
	default:
		if s.OnStatus != nil {
			s.OnStatus(fmt.Sprintf("warn: dropped unknown render kind %q", ri.Kind))
		}
	}
}

func (s *Server) dispatchPanel(from string, f *protocol.Frame) {
	var op PanelOp
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &op); err != nil {
			s.rejectPanel(from, "malformed payload")
			return
		}
	}
	if err := s.validatePanelOp(from, op); err != nil {
		s.rejectPanel(from, err.Error())
		return
	}
	s.publish(Event{Topic: "panel", Data: op})
}

// validatePanelOp enforces the Panel Component contract (ADR-0010): the
// component tag must belong to the emitting plugin and target a real slot.
func (s *Server) validatePanelOp(from string, op PanelOp) error {
	if op.Op != "set" && op.Op != "clear" {
		return fmt.Errorf("op %q must be set|clear", op.Op)
	}
	if op.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !plugin.ValidMountSlot(op.Slot) {
		return fmt.Errorf("unknown slot %q", op.Slot)
	}
	if op.Op == "set" {
		if op.Component == "" {
			return fmt.Errorf("component is required for set")
		}
		if !plugin.ValidComponentTag(op.Component) {
			return fmt.Errorf("component %q must be a valid custom element tag", op.Component)
		}
		if !strings.HasPrefix(op.Component, from+"-") {
			return fmt.Errorf("component %q must be prefixed with %q", op.Component, from+"-")
		}
	}
	if len(op.Props) > 0 {
		trimmed := strings.TrimSpace(string(op.Props))
		if !json.Valid(op.Props) || !strings.HasPrefix(trimmed, "{") {
			return fmt.Errorf("props must be a JSON object")
		}
	}
	return nil
}

// rejectPanel suppresses the op and surfaces the violation instead of
// silently dropping it. Panel evt frames carry no id, so there is no
// correlated error Frame; the warn status is the observable rejection.
func (s *Server) rejectPanel(from, reason string) {
	if s.OnStatus != nil {
		s.OnStatus(fmt.Sprintf("warn: rejected panel op from %s: %s", from, reason))
	}
	s.publish(Event{Topic: "status", Data: map[string]string{
		"status": fmt.Sprintf("warn: rejected panel op from %s: %s", from, reason),
	}})
}

func (s *Server) routeRequest(from string, f *protocol.Frame) {
	// Host closed: reject star calls without an interceptor plane (ADR-0014).
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		_ = s.writeTo(from, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
			Error: &protocol.FrameError{Code: "host_closed", Message: "host_closed"},
		})
		return
	}

	// Host owns agent/request (log invariant) and agent.inject (append-only notify).
	if f.Cap == AgentCap && (f.Method == "request" || f.Method == "inject" || f.Method == "confirm") {
		s.handleAgentFromPlugin(from, f)
		return
	}

	// Host owns ensurePlugins (ADR-0023).
	if f.Cap == HostCap && f.Method == "ensurePlugins" {
		s.handleEnsurePlugins(from, f)
		return
	}

	// Multi-provider tools (ADR-0018): merge list / route call by tool name.
	if f.Cap == ToolsCap {
		s.routeToolsFromPlugin(from, f)
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	owner, ok := s.provides[f.Cap]
	if !ok || owner == from {
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "capability_unavailable", Message: fmt.Sprintf("unknown capability %q", f.Cap)},
		})
		return
	}
	to := DefaultCallTimeout
	if p := s.plugins[owner]; p != nil {
		to = p.timeout
	}
	s.seq++
	fwdID := fmt.Sprintf("fwd-%d", s.seq)
	s.pending[fwdID] = &wait{
		kind:       waitPlugin,
		caller:     from,
		target:     owner,
		origID:     f.ID,
		cap:        f.Cap,
		method:     f.Method,
		reqPayload: append(json.RawMessage(nil), f.Payload...),
	}
	s.mu.Unlock()

	if err := s.ensureAlive(owner); err != nil {
		s.mu.Lock()
		delete(s.pending, fwdID)
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "route_failed", Message: err.Error()},
		})
		return
	}

	fwd := *f
	fwd.ID = fwdID
	if err := s.writeTo(owner, &fwd); err != nil {
		s.mu.Lock()
		delete(s.pending, fwdID)
		s.mu.Unlock()
		_ = s.writeTo(from, &protocol.Frame{
			Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "route_failed", Message: err.Error()},
		})
		return
	}

	timer := time.AfterFunc(to, func() {
		s.mu.Lock()
		w, ok := s.pending[fwdID]
		if ok {
			delete(s.pending, fwdID)
		}
		s.mu.Unlock()
		if !ok {
			return
		}
		_ = s.writeTo(from, &protocol.Frame{
			Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
			Error: &protocol.FrameError{Code: "timeout", Message: fmt.Sprintf("call to %s timed out after %s", owner, to)},
		})
		_ = w
	})
	// timer stopped in complete when pending is removed; leak-once acceptable for lite host if not stopped.
	_ = timer
}

// routeToolsFromPlugin handles star-routed tools.list/call with multi-provider merge (ADR-0018).
func (s *Server) routeToolsFromPlugin(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
	}
	switch f.Method {
	case "list":
		payload, err := s.toolsListMerged()
		if err != nil {
			res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
		} else {
			res.Payload = payload
		}
		_ = s.writeTo(from, res)
	case "call":
		var in struct {
			Name string `json:"name"`
		}
		if len(f.Payload) > 0 {
			_ = json.Unmarshal(f.Payload, &in)
		}
		owner, ok := s.toolsOwnerFor(in.Name)
		if !ok {
			res.Error = &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
			_ = s.writeTo(from, res)
			return
		}
		if owner == from {
			// Self-call of an owned tool: execute via Call (bypass star self-block).
			payload, err := s.CallByPlugin(owner, ToolsCap, "call", f.Payload)
			if err != nil {
				res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
			} else {
				res.Payload = payload
			}
			_ = s.writeTo(from, res)
			return
		}
		// Forward like a normal star call to the owning tools Plugin.
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		to := DefaultCallTimeout
		if p := s.plugins[owner]; p != nil {
			to = p.timeout
		}
		s.seq++
		fwdID := fmt.Sprintf("fwd-%d", s.seq)
		s.pending[fwdID] = &wait{
			kind:       waitPlugin,
			caller:     from,
			target:     owner,
			origID:     f.ID,
			cap:        f.Cap,
			method:     f.Method,
			reqPayload: append(json.RawMessage(nil), f.Payload...),
		}
		s.mu.Unlock()
		if err := s.ensureAlive(owner); err != nil {
			s.mu.Lock()
			delete(s.pending, fwdID)
			s.mu.Unlock()
			res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
		fwd := *f
		fwd.ID = fwdID
		if err := s.writeTo(owner, &fwd); err != nil {
			s.mu.Lock()
			delete(s.pending, fwdID)
			s.mu.Unlock()
			res.Error = &protocol.FrameError{Code: "route_failed", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
		timer := time.AfterFunc(to, func() {
			s.mu.Lock()
			w, ok := s.pending[fwdID]
			if ok {
				delete(s.pending, fwdID)
			}
			s.mu.Unlock()
			if !ok {
				return
			}
			_ = s.writeTo(from, &protocol.Frame{
				Type: protocol.TypeRes, ID: f.ID, Cap: f.Cap,
				Error: &protocol.FrameError{Code: "timeout", Message: fmt.Sprintf("call to %s timed out after %s", owner, to)},
			})
			_ = w
		})
		_ = timer
	default:
		res.Error = &protocol.FrameError{Code: "method_not_found", Message: "unknown tools." + f.Method}
		_ = s.writeTo(from, res)
	}
}

func (s *Server) complete(f *protocol.Frame) {
	debugf("complete id=%s type=%s cap=%s method=%s", f.ID, f.Type, f.Cap, f.Method)
	s.mu.Lock()
	w, ok := s.pending[f.ID]
	if ok {
		delete(s.pending, f.ID)
	}
	var evts []*protocol.Frame
	if ok && w != nil {
		evts = w.events
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if w.kind == waitHost {
		select {
		case w.ch <- &CallResult{Frame: f, Events: evts}:
		default:
		}
		return
	}
	// Fan-out plugin-originated session.append so Web trace stays live
	// without Host interpreting fact types (reasoning, tools, …).
	if f.Error == nil && w.cap == SessionCap && w.method == "append" {
		s.publishPluginSessionAppend(w.reqPayload, f)
	}
	// Host policy: every llm.complete that reports usage is mirrored into
	// Context Manager so tokens stay accurate even if a custom Loop forgets.
	// Async: must not block the LLM Plugin's readLoop on another star call.
	if f.Error == nil && w.cap == LLMCap && w.method == "complete" {
		reqPayload, resPayload := w.reqPayload, f.Payload
		go s.noteLLMUsage(reqPayload, resPayload)
	}
	out := *f
	out.ID = w.origID
	_ = s.writeTo(w.caller, &out)
}

// noteLLMUsage forwards provider usage from an llm.complete res into Context
// Manager. sessionId comes from the original request payload.
func (s *Server) noteLLMUsage(reqPayload, resPayload json.RawMessage) {
	var req struct {
		SessionID string `json:"sessionId"`
	}
	if len(reqPayload) > 0 {
		_ = json.Unmarshal(reqPayload, &req)
	}
	var out struct {
		Usage map[string]any `json:"usage"`
	}
	if len(resPayload) > 0 {
		_ = json.Unmarshal(resPayload, &out)
	}
	if req.SessionID == "" || len(out.Usage) == 0 {
		return
	}
	_ = s.NoteContextUsage(req.SessionID, out.Usage)
}

// publishPluginSessionAppend mirrors AppendSessionFacts' live topic=session event
// for appends that came from a Plugin (star call), not from the Host Loop.
func (s *Server) publishPluginSessionAppend(reqPayload json.RawMessage, res *protocol.Frame) {
	if len(reqPayload) == 0 {
		return
	}
	var body map[string]any
	if err := json.Unmarshal(reqPayload, &body); err != nil {
		return
	}
	var out struct {
		Seq int `json:"seq"`
	}
	if len(res.Payload) > 0 {
		_ = json.Unmarshal(res.Payload, &out)
	}
	factOut := make(map[string]any, len(body)+1)
	for k, v := range body {
		factOut[k] = v
	}
	factOut["seq"] = out.Seq
	s.publish(Event{Topic: "session", Data: factOut})
}

func (s *Server) writeTo(plugin string, f *protocol.Frame) error {
	s.mu.Lock()
	p := s.plugins[plugin]
	closed := s.closed
	s.mu.Unlock()
	if closed || p == nil {
		return fmt.Errorf("plugin %s not connected", plugin)
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	debugFrame("->", plugin, f)
	return protocol.WriteFrame(p.stdin, f)
}

// Call sends a Host-initiated req to pluginName and waits for its res (with timeout).
// Retries once on plugin_down after on-demand restart (crash recovery).
func (s *Server) Call(pluginName string, f *protocol.Frame) (*protocol.Frame, error) {
	out, err := s.CallStream(pluginName, f)
	if err != nil {
		return nil, err
	}
	return out.Frame, nil
}

// CallCommand routes a slash command into pluginName (cap=commands, method=call).
func (s *Server) CallCommand(pluginName, command, args string) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]string{
		"command": command,
		"args":    args,
	})
	if err != nil {
		return nil, err
	}
	out, err := s.Call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     CommandsCap,
		Method:  CommandsCallMethod,
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

// CallUIAction routes a UI Action into pluginName (cap=ui, method=action).
func (s *Server) CallUIAction(pluginName string, payload json.RawMessage) (json.RawMessage, error) {
	out, err := s.Call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     UICap,
		Method:  UIActionMethod,
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

// CallByCap routes cap.method to the Plugin that provides cap.
// For tools (multi-provider, ADR-0018), list merges and call routes by tool name.
func (s *Server) CallByCap(cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	if cap == ToolsCap {
		if method == "list" {
			return s.toolsListMerged()
		}
		if method == "call" {
			var in struct {
				Name string `json:"name"`
			}
			if len(payload) > 0 {
				_ = json.Unmarshal(payload, &in)
			}
			owner, ok := s.toolsOwnerFor(in.Name)
			if !ok {
				return nil, &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
			}
			return s.CallByPlugin(owner, ToolsCap, method, payload)
		}
	}
	s.mu.Lock()
	owner, ok := s.provides[cap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", cap)
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	out, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     cap,
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

// CallStream is Call plus any evt frames attributed to the request id while waiting.
func (s *Server) CallStream(pluginName string, f *protocol.Frame) (*CallResult, error) {
	return s.CallStreamOn(pluginName, f, nil)
}

// CallStreamOn is CallStream with a live callback for each attributed evt (Render Medium).
// Callback runs on the Host read-loop goroutine; keep it fast and non-blocking.
func (s *Server) CallStreamOn(pluginName string, f *protocol.Frame, onEvent func(*protocol.Frame)) (*CallResult, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			if err := s.ensureAlive(pluginName); err != nil {
				return nil, lastErr
			}
		} else if err := s.ensureAlive(pluginName); err != nil {
			return nil, err
		}
		res, err := s.callOnce(pluginName, f, onEvent)
		if err != nil {
			lastErr = err
			continue
		}
		if res.Frame.Error != nil && res.Frame.Error.Code == "plugin_down" && attempt == 0 {
			lastErr = fmt.Errorf("%s", res.Frame.Error.Error())
			continue
		}
		return res, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("call failed")
	}
	return nil, lastErr
}

func (s *Server) callOnce(pluginName string, f *protocol.Frame, onEvent func(*protocol.Frame)) (*CallResult, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("server closed")
	}
	to := DefaultCallTimeout
	if p := s.plugins[pluginName]; p != nil {
		to = p.timeout
	}
	s.seq++
	id := fmt.Sprintf("host-%d", s.seq)
	f.ID = id
	ch := make(chan *CallResult, 1)
	s.pending[id] = &wait{kind: waitHost, target: pluginName, ch: ch, onEvent: onEvent}
	s.mu.Unlock()

	if err := s.writeTo(pluginName, f); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(to):
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("timeout: call to %s timed out after %s", pluginName, to)
	}
}

// Close shuts down every Plugin: stdin EOF, grace wait, then kill tree.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	debugf("close host plugins=%d", len(s.plugins))
	plugins := make([]*proc, 0, len(s.plugins))
	for _, p := range s.plugins {
		plugins = append(plugins, p)
	}
	job := s.job
	s.job = nil
	s.mu.Unlock()

	for _, p := range plugins {
		p.wmu.Lock()
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		p.wmu.Unlock()
	}
	deadline := time.Now().Add(DefaultShutdownGrace)
	for _, p := range plugins {
		if p.cmd == nil || p.cmd.Process == nil {
			continue
		}
		done := make(chan error, 1)
		go func(cmd *exec.Cmd) { done <- cmd.Wait() }(p.cmd)
		remain := time.Until(deadline)
		if remain < 0 {
			remain = 0
		}
		select {
		case <-done:
		case <-time.After(remain):
			killTree(p.cmd)
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}
	// Closing the Job Object (KILL_ON_JOB_CLOSE) reaps any stragglers,
	// including when Host itself is dying without a clean Close of each child.
	if job != nil {
		job.close()
	}
	return nil
}

// MarshalPayload JSON-encodes v for a Call payload.
func MarshalPayload(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// Message is one model-visible chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// SessionCap is the Capability name Session Plugins must provide.
const SessionCap = "session"

// AgentCap is the Host-owned Capability namespace for agent/request and agent.inject.
const AgentCap = "agent"

// LLMCap is the Capability name LLM Plugins provide (consumed by Agent Loop).
const LLMCap = "llm"

// SystemPromptCap is the Capability Context Manager Plugins provide (ADR-0006).
const SystemPromptCap = "system-prompt"

// ContextCap is the Capability Context Manager Plugins provide for prepare/compact/usage (ADR-0013).
const ContextCap = "context"

// LoopCap is the Agent Loop Capability (ADR-0016). A mounted Agent Plugin must
// provide it; Host has no in-process default Loop.
const LoopCap = "loop"

// LLMChunkMethod is the evt method LLM Plugins use to stream a delta.
const LLMChunkMethod = "chunk"

// LLMCompleteMethod is the req/res method LLM Plugins implement.
const LLMCompleteMethod = "complete"

// ToolsCap is the Capability Tools Plugins provide (list/call).
const ToolsCap = "tools"

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// TurnResult is one Agent Loop turn returned by the mounted loop provider (ADR-0016).
type TurnResult struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Chunks    []string  `json:"chunks"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
	Messages  []Message `json:"messages"`
}

// AgentRequestResult is the validated Model Context after the log invariant check.
type AgentRequestResult struct {
	Messages []Message `json:"messages"`
	// Rebuilt is true when Host rebuilt Model Context from the log (empty claimed).
	// When claimed was supplied and matched, Rebuilt is false (validated, not rebuilt).
	Rebuilt bool `json:"rebuilt"`
}

// QuerySessionFacts lists Session Log facts via session.query.
// Empty sessionID targets the default Session.
func (s *Server) QuerySessionFacts(sessionID string, afterSeq, limit int) ([]map[string]any, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	payload := map[string]any{"afterSeq": afterSeq, "limit": limit}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "query",
		Payload: MarshalPayload(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("session.query: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("session.query: %w", res.Error)
	}
	var out struct {
		Facts []map[string]any `json:"facts"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil {
		return nil, fmt.Errorf("session.query: bad payload: %w", err)
	}
	if out.Facts == nil {
		out.Facts = []map[string]any{}
	}
	return out.Facts, nil
}

// DeriveMessages rebuilds Model Context from the mounted Session Plugin (session.derive).
// Empty sessionID targets the default Session.
func (s *Server) DeriveMessages(sessionID string) ([]Message, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	payload := json.RawMessage(`{}`)
	if sessionID != "" {
		payload = MarshalPayload(map[string]string{"sessionId": sessionID})
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "derive",
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("session.derive: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("session.derive: %w", res.Error)
	}
	var out struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil {
		return nil, fmt.Errorf("session.derive: bad payload: %w", err)
	}
	if out.Messages == nil {
		out.Messages = []Message{}
	}
	return out.Messages, nil
}

// AppendSessionFacts appends each fact to the mounted Session Plugin.
// Empty sessionID targets the default Session.
func (s *Server) AppendSessionFacts(sessionID string, facts []map[string]any) (int, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	last := 0
	for _, fact := range facts {
		body := fact
		if sessionID != "" {
			body = make(map[string]any, len(fact)+1)
			for k, v := range fact {
				body[k] = v
			}
			body["sessionId"] = sessionID
		}
		res, err := s.Call(owner, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     SessionCap,
			Method:  "append",
			Payload: MarshalPayload(body),
		})
		if err != nil {
			return last, fmt.Errorf("session.append: %w", err)
		}
		if res.Error != nil {
			return last, fmt.Errorf("session.append: %w", res.Error)
		}
		var out struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return last, fmt.Errorf("session.append: bad payload: %w", err)
		}
		last = out.Seq
		// Live Session Log for Web trace (topic=session).
		factOut := make(map[string]any, len(body)+2)
		for k, v := range body {
			factOut[k] = v
		}
		if _, ok := factOut["sessionId"]; !ok {
			factOut["sessionId"] = sessionID
		}
		factOut["seq"] = out.Seq
		s.publish(Event{Topic: "session", Data: factOut})
	}
	return last, nil
}

// AgentRequest enforces the Session Log invariant before a model call (ADR-0002).
//
// claimed empty/nil → rebuild from session.derive and accept.
// claimed non-empty → must match session.derive exactly, else session_invariant_violation.
// Empty sessionID targets the default Session.
func (s *Server) AgentRequest(sessionID string, claimed []Message) (*AgentRequestResult, error) {
	derived, err := s.DeriveMessages(sessionID)
	if err != nil {
		return nil, fmt.Errorf("agent/request: %w", err)
	}
	if len(claimed) > 0 && !messagesEqual(claimed, derived) {
		return nil, &protocol.FrameError{
			Code:    "session_invariant_violation",
			Message: "model context is not reconstructable from session log",
		}
	}
	return &AgentRequestResult{Messages: derived, Rebuilt: len(claimed) == 0}, nil
}

// ContextUsage returns the Context Manager's last prepare usage for a session.
func (s *Server) ContextUsage(sessionID string) (map[string]any, error) {
	s.mu.Lock()
	owner, ok := s.provides[ContextCap]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	payload := json.RawMessage(`{}`)
	if sessionID != "" {
		payload = MarshalPayload(map[string]string{"sessionId": sessionID})
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     ContextCap,
		Method:  "usage",
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("context.usage: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("context.usage: %w", res.Error)
	}
	var out struct {
		Usage map[string]any `json:"usage"`
	}
	if len(res.Payload) > 0 {
		_ = json.Unmarshal(res.Payload, &out)
	}
	return out.Usage, nil
}

// NoteContextUsage pushes provider usage into Context Manager (source=provider).
func (s *Server) NoteContextUsage(sessionID string, usage map[string]any) error {
	s.mu.Lock()
	owner, ok := s.provides[ContextCap]
	s.mu.Unlock()
	if !ok || len(usage) == 0 {
		return nil
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:      protocol.Version,
		Type:   protocol.TypeReq,
		Cap:    ContextCap,
		Method: "noteUsage",
		Payload: MarshalPayload(map[string]any{
			"sessionId": sessionID,
			"usage":     usage,
		}),
	})
	if err != nil {
		return fmt.Errorf("context.noteUsage: %w", err)
	}
	if res.Error != nil {
		return fmt.Errorf("context.noteUsage: %w", res.Error)
	}
	return nil
}

// ListContextMessages returns the last prepare messages preview from Context Manager.
func (s *Server) ListContextMessages(sessionID string, n int) ([]Message, error) {
	s.mu.Lock()
	owner, ok := s.provides[ContextCap]
	s.mu.Unlock()
	if !ok {
		return nil, nil
	}
	payload := MarshalPayload(map[string]any{"sessionId": sessionID, "n": n})
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     ContextCap,
		Method:  "listContext",
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("context.listContext: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("context.listContext: %w", res.Error)
	}
	var out struct {
		Messages []Message `json:"messages"`
	}
	if len(res.Payload) > 0 {
		_ = json.Unmarshal(res.Payload, &out)
	}
	if out.Messages == nil {
		out.Messages = []Message{}
	}
	return out.Messages, nil
}

// sessionTurn is one Session's in-flight default-Loop turn control.
// mu serializes turns on the same Session; different Sessions run in parallel.
type sessionTurn struct {
	mu      sync.Mutex
	cancel  atomic.Bool
	running atomic.Bool
}

// normalizeSessionID maps empty/blank to the default Session id so Host turn
// state, status, and render tags never split "" vs "default" into two Sessions.
func normalizeSessionID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "default"
	}
	return id
}

func (s *Server) turnFor(sid string) *sessionTurn {
	sid = normalizeSessionID(sid)
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	if s.turnStates == nil {
		s.turnStates = make(map[string]*sessionTurn)
	}
	st := s.turnStates[sid]
	if st == nil {
		st = &sessionTurn{}
		s.turnStates[sid] = st
	}
	return st
}

// acquireTurn TryLocks the Session turn. Returns nil if that Session is busy.
func (s *Server) acquireTurn(sid string) *sessionTurn {
	st := s.turnFor(sid)
	if !st.mu.TryLock() {
		return nil
	}
	st.cancel.Store(false)
	return st
}

func (s *Server) beginRunning(sid string) {
	s.turnFor(sid).running.Store(true)
}

func (s *Server) endRunning(sid string) {
	s.turnFor(sid).running.Store(false)
}

// IsRunning reports whether any default-Loop turn is in flight.
func (s *Server) IsRunning() bool {
	return len(s.RunningSessions()) > 0
}

// IsRunningOn reports whether sid itself has an in-flight turn.
func (s *Server) IsRunningOn(sid string) bool {
	sid = normalizeSessionID(sid)
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	st := s.turnStates[sid]
	return st != nil && st.running.Load()
}

// RunningSessions lists Session ids with an in-flight turn.
func (s *Server) RunningSessions() []string {
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	var out []string
	for sid, st := range s.turnStates {
		if st.running.Load() {
			out = append(out, sid)
		}
	}
	return out
}

// StatusForSession reports "running" when sid owns an in-flight turn, else "idle".
func (s *Server) StatusForSession(sid string) string {
	if s.IsRunningOn(sid) {
		return "running"
	}
	return "idle"
}

func (s *Server) emitStatus(sessionID, status string) {
	sessionID = normalizeSessionID(sessionID)
	if s.OnStatus != nil {
		s.OnStatus(status)
	}
	s.publish(Event{Topic: "status", Data: map[string]string{"status": status, "sessionId": sessionID}})
}

// extractStreamDelta returns text delta, channel (content|reasoning), and
// sessionId from llm.chunk or presentation.stream chunk frames.
func extractStreamDelta(f *protocol.Frame) (delta, channel, sessionID string, ok bool) {
	var c struct {
		Op        string `json:"op"`
		Delta     string `json:"delta"`
		Channel   string `json:"channel"`
		SessionID string `json:"sessionId"`
	}
	if len(f.Payload) > 0 {
		_ = json.Unmarshal(f.Payload, &c)
	}
	if c.Channel == "" {
		c.Channel = "content"
	}
	if f.Method == LLMChunkMethod {
		return c.Delta, c.Channel, c.SessionID, c.Delta != ""
	}
	if f.Cap == PresentationCap && f.Method == PresentationStreamMethod && c.Op == "chunk" {
		return c.Delta, c.Channel, c.SessionID, c.Delta != ""
	}
	return "", "", "", false
}

// RunTurn is the Host entry for one chat turn: lock + status + loop.turn (ADR-0016).
//
// RunTurn is the Host entry for one chat turn: lock + status + loop.turn (ADR-0016).
// Default flow: turn/start → system-prompt.assemble → session.append(system) → session.append(user)
// → per Step: step/start → AgentRequest(rebuild) → llm.complete(tools)
// → optional tools.call → session tool facts → step/end → repeat until final assistant reply
// → turn/end.
func (s *Server) RunTurn(userInput string) (*TurnResult, error) {
	return s.RunTurnOn("", userInput)
}

// RunTurnOn is RunTurn bound to a Session id (empty = default).
// Turns on different Sessions run in parallel; the same Session stays serial.
func (s *Server) RunTurnOn(sessionID, userInput string) (*TurnResult, error) {
	return s.runTurn(normalizeSessionID(sessionID), userInput, true, "")
}

// CancelTurnOn requests the in-flight Loop on sessionID to stop at the next safe boundary.
// Also notifies the mounted Agent Plugin so an external Loop can observe cancel (ADR-0016).
func (s *Server) CancelTurnOn(sessionID string) {
	sessionID = normalizeSessionID(sessionID)
	s.turnStatesMu.Lock()
	st := s.turnStates[sessionID]
	s.turnStatesMu.Unlock()
	if st != nil {
		st.cancel.Store(true)
	}
	s.mu.Lock()
	loopOwner, ok := s.provides[LoopCap]
	s.mu.Unlock()
	if ok {
		payload := MarshalPayload(map[string]any{"sessionId": sessionID})
		_ = s.writeTo(loopOwner, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     LoopCap,
			Method:  "cancel",
			Payload: payload,
		})
	}
}

// CancelTurn cancels every in-flight turn (CLI Ctrl+C / legacy).
func (s *Server) CancelTurn() {
	s.turnStatesMu.Lock()
	states := make([]*sessionTurn, 0, len(s.turnStates))
	for _, st := range s.turnStates {
		states = append(states, st)
	}
	s.turnStatesMu.Unlock()
	for _, st := range states {
		st.cancel.Store(true)
	}
}

// TurnCancelledOn reports whether CancelTurnOn was requested for sessionID.
func (s *Server) TurnCancelledOn(sessionID string) bool {
	sessionID = normalizeSessionID(sessionID)
	s.turnStatesMu.Lock()
	st := s.turnStates[sessionID]
	s.turnStatesMu.Unlock()
	return st != nil && st.cancel.Load()
}

// TurnCancelled reports whether any turn has a cancel request.
func (s *Server) TurnCancelled() bool {
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	for _, st := range s.turnStates {
		if st.cancel.Load() {
			return true
		}
	}
	return false
}

// ListSessions returns mounted Session Plugin session ids (for Web history rail).
func (s *Server) ListSessions() ([]map[string]any, error) {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", SessionCap)
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     SessionCap,
		Method:  "list",
		Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		return nil, fmt.Errorf("session.list: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("session.list: %w", res.Error)
	}
	var out struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if len(res.Payload) > 0 {
		_ = json.Unmarshal(res.Payload, &out)
	}
	if out.Sessions == nil {
		out.Sessions = []map[string]any{}
	}
	return out.Sessions, nil
}

// CreateSession creates a Session by id via the mounted Session Plugin (idempotent).
func (s *Server) CreateSession(sessionID, parentSession, origin string, delegationDepth int) error {
	return s.CreateSessionWithWorkspace(sessionID, parentSession, origin, delegationDepth, "")
}

// CreateSessionWithWorkspace creates/updates a Session, optionally setting Workspace (ADR-0020).
func (s *Server) CreateSessionWithWorkspace(sessionID, parentSession, origin string, delegationDepth int, workspace string) error {
	s.mu.Lock()
	owner, ok := s.provides[SessionCap]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no plugin provides %q", SessionCap)
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:      protocol.Version,
		Type:   protocol.TypeReq,
		Cap:    SessionCap,
		Method: "create",
		Payload: MarshalPayload(map[string]any{
			"sessionId":       sessionID,
			"parentSession":   parentSession,
			"origin":          origin,
			"delegationDepth": delegationDepth,
			"workspace":       workspace,
		}),
	})
	if err != nil {
		return fmt.Errorf("session.create: %w", err)
	}
	if res.Error != nil {
		return fmt.Errorf("session.create: %w", res.Error)
	}
	return nil
}

// SessionWorkspace returns the Workspace bound to a Session (empty when unset).
func (s *Server) SessionWorkspace(sessionID string) string {
	payload, err := s.CallByCap(SessionCap, "info", MarshalPayload(map[string]any{"sessionId": sessionID}))
	if err != nil {
		return ""
	}
	var out struct {
		Workspace string `json:"workspace"`
	}
	_ = json.Unmarshal(payload, &out)
	return out.Workspace
}

// SetSessionWorkspace upserts Workspace on an existing Session (ADR-0020).
func (s *Server) SetSessionWorkspace(sessionID, workspace string) error {
	return s.CreateSessionWithWorkspace(sessionID, "", "", 0, workspace)
}

// NewSessionIDWithWorkspace creates a Session with an optional Workspace root.
func (s *Server) NewSessionIDWithWorkspace(id, workspace string) (string, error) {
	if id == "" {
		id = fmt.Sprintf("s-%d", time.Now().UnixNano())
	}
	if err := s.CreateSessionWithWorkspace(id, "", "web", 0, workspace); err != nil {
		return "", err
	}
	return id, nil
}

// runTurn executes one Turn on sessionID via the mounted Agent Plugin (ADR-0016).
// Host owns the per-session lock, Cancel/running status; Loop policy lives in the agent.
func (s *Server) runTurn(sessionID, userInput string, allowSubagent bool, extraSystem string) (result *TurnResult, err error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, fmt.Errorf("agent loop: user input is required")
	}

	st := s.acquireTurn(sessionID)
	if st == nil {
		return nil, fmt.Errorf("session already has a running turn")
	}
	defer st.mu.Unlock()

	s.mu.Lock()
	loopOwner, hasExternalLoop := s.provides[LoopCap]
	s.mu.Unlock()
	if !hasExternalLoop {
		return nil, fmt.Errorf("agent loop: no plugin provides %q (mount an Agent plugin)", LoopCap)
	}

	s.beginRunning(sessionID)
	s.emitStatus(sessionID, "running")
	status := "idle"
	defer func() {
		s.endRunning(sessionID)
		s.emitStatus(sessionID, status)
	}()

	res, err := s.runExternalTurn(loopOwner, sessionID, userInput, allowSubagent, extraSystem)
	if err != nil {
		if st.cancel.Load() {
			status = "idle"
			return nil, fmt.Errorf("turn cancelled")
		}
		status = "error"
		if err.Error() != "" {
			status = "error: " + err.Error()
		}
		return nil, err
	}
	return res, nil
}

func (s *Server) runExternalTurn(owner, sessionID, userInput string, allowSubagent bool, extraSystem string) (*TurnResult, error) {
	payload := map[string]any{"input": userInput, "allowSubagent": allowSubagent}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	if extraSystem != "" {
		payload["extraSystem"] = extraSystem
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     LoopCap,
		Method:  "turn",
		Payload: MarshalPayload(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("agent loop: %w", res.Error)
	}
	var out TurnResult
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return nil, fmt.Errorf("agent loop: bad loop.turn payload: %w", err)
		}
	}
	if out.User == "" {
		out.User = userInput
	}
	return &out, nil
}

func messagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
		if a[i].ToolCallID != b[i].ToolCallID {
			return false
		}
		if len(a[i].ToolCalls) != len(b[i].ToolCalls) {
			return false
		}
		for j := range a[i].ToolCalls {
			x, y := a[i].ToolCalls[j], b[i].ToolCalls[j]
			if x.ID != y.ID || x.Name != y.Name || !bytesEqualJSON(x.Arguments, y.Arguments) {
				return false
			}
		}
	}
	return true
}

func bytesEqualJSON(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return string(a) == string(b)
}

// handleEnsurePlugins mounts discovered plugins by name (and their dependsOn closure).
func (s *Server) handleEnsurePlugins(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method,
	}
	var in struct {
		Names []string `json:"names"`
	}
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &in); err != nil {
			res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			_ = s.writeTo(from, res)
			return
		}
	}
	out, err := s.EnsurePlugins(in.Names)
	if err != nil {
		res.Error = &protocol.FrameError{Code: "ensure_plugins_failed", Message: err.Error()}
		_ = s.writeTo(from, res)
		return
	}
	res.Payload = MarshalPayload(out)
	_ = s.writeTo(from, res)
}

// EnsurePluginsResult is the ensurePlugins response (ADR-0023).
type EnsurePluginsResult struct {
	Mounted []string `json:"mounted"`
	Missing []string `json:"missing"`
	Failed  []string `json:"failed,omitempty"`
}

// EnsurePlugins idempotently mounts catalog plugins by name (UI-only allowed).
// The dependsOn closure is expanded by assembly.ResolveClosure (ADR-0021).
func (s *Server) EnsurePlugins(names []string) (*EnsurePluginsResult, error) {
	catalog := s.ensureCatalog()
	out := &EnsurePluginsResult{}

	mountedSet, missing, _ := assembly.ResolveClosure(catalog, names)
	out.Missing = missing

	var toLaunch []discovery.Found
	for _, p := range mountedSet {
		s.mu.Lock()
		_, alive := s.plugins[p.Manifest.Name]
		_, ui := s.mountedUI[p.Manifest.Name]
		s.mu.Unlock()
		if alive || ui {
			out.Mounted = append(out.Mounted, p.Manifest.Name)
			continue
		}
		toLaunch = append(toLaunch, p)
	}

	if len(toLaunch) == 0 {
		return out, nil
	}
	if err := s.registerProvides(toLaunch); err != nil {
		return nil, err
	}
	for _, p := range toLaunch {
		if p.Manifest.Entry == "" {
			s.mu.Lock()
			s.mountedUI[p.Manifest.Name] = true
			s.mu.Unlock()
			out.Mounted = append(out.Mounted, p.Manifest.Name)
			continue
		}
		if err := s.launch(p); err != nil {
			out.Failed = append(out.Failed, p.Manifest.Name)
			fmt.Fprintf(os.Stderr, "ensurePlugins: launch %s: %v\n", p.Manifest.Name, err)
			continue
		}
		out.Mounted = append(out.Mounted, p.Manifest.Name)
	}
	s.reconcileConsumes()
	if err := s.discoverTools(); err != nil {
		fmt.Fprintf(os.Stderr, "ensurePlugins: discoverTools: %v\n", err)
	}
	return out, nil
}

// handleAgentFromPlugin serves Host-owned agent.request and agent.inject.
func (s *Server) handleAgentFromPlugin(from string, f *protocol.Frame) {
	res := &protocol.Frame{
		V:      f.V,
		ID:     f.ID,
		Type:   protocol.TypeRes,
		Cap:    f.Cap,
		Method: f.Method,
	}
	switch f.Method {
	case "request":
		var in struct {
			SessionID string    `json:"sessionId"`
			Messages  []Message `json:"messages"`
		}
		if len(f.Payload) > 0 {
			if err := json.Unmarshal(f.Payload, &in); err != nil {
				res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
				_ = s.writeTo(from, res)
				return
			}
		}
		out, err := s.AgentRequest(in.SessionID, in.Messages)
		if err != nil {
			if fe, ok := err.(*protocol.FrameError); ok {
				res.Error = fe
			} else {
				res.Error = &protocol.FrameError{Code: "agent_request_failed", Message: err.Error()}
			}
			_ = s.writeTo(from, res)
			return
		}
		res.Payload = MarshalPayload(out)
		_ = s.writeTo(from, res)
	case "inject":
		out, err := s.AgentInject(f.Payload)
		if err != nil {
			if fe, ok := err.(*protocol.FrameError); ok {
				res.Error = fe
			} else {
				res.Error = &protocol.FrameError{Code: "agent_inject_failed", Message: err.Error()}
			}
			_ = s.writeTo(from, res)
			return
		}
		res.Payload = MarshalPayload(out)
		_ = s.writeTo(from, res)
	case "confirm":
		var in struct {
			Tool        string          `json:"tool"`
			Arguments   json.RawMessage `json:"arguments"`
			Workspace   string          `json:"workspace"`
			SessionID   string          `json:"sessionId"`
			Description string          `json:"description"`
		}
		if len(f.Payload) > 0 {
			if err := json.Unmarshal(f.Payload, &in); err != nil {
				res.Error = &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
				_ = s.writeTo(from, res)
				return
			}
		}
		approved := false
		if s.OnToolApproval != nil {
			approved = s.OnToolApproval(in.Tool, in.Arguments, in.Workspace, in.SessionID)
		}
		res.Payload = MarshalPayload(map[string]any{"approved": approved})
		_ = s.writeTo(from, res)
	default:
		res.Error = &protocol.FrameError{
			Code:    "method_not_found",
			Message: fmt.Sprintf("no handler for agent.%s", f.Method),
		}
		_ = s.writeTo(from, res)
	}
}

// AgentInject appends model-visible messages to the Session Log without starting a turn (US17).
// Accepts {"role","content"} or {"messages":[{role,content},...]} or optional sessionId.
func (s *Server) AgentInject(payload json.RawMessage) (map[string]int, error) {
	var in struct {
		SessionID string    `json:"sessionId"`
		Role      string    `json:"role"`
		Content   string    `json:"content"`
		Messages  []Message `json:"messages"`
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
	}
	msgs := in.Messages
	if len(msgs) == 0 && (in.Role != "" || in.Content != "") {
		msgs = []Message{{Role: in.Role, Content: in.Content}}
	}
	if len(msgs) == 0 {
		return nil, &protocol.FrameError{Code: "bad_payload", Message: "inject requires role/content or messages"}
	}
	facts := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		facts = append(facts, map[string]any{
			"type":    "message",
			"role":    defaultSystemRole(m.Role),
			"content": m.Content,
		})
	}
	seq, err := s.AppendSessionFacts(in.SessionID, facts)
	if err != nil {
		return nil, err
	}
	return map[string]int{"count": len(facts), "lastSeq": seq}, nil
}

func defaultSystemRole(role string) string {
	if role == "" {
		return "system"
	}
	return role
}
