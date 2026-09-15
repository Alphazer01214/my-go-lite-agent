// Package web is the Host-embedded Web Render Medium (ADR-0009).
package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/serve"
)

// DefaultReplay is the in-memory ring buffer size for late SSE subscribers.
const DefaultReplay = 500

// Options configures the Web Medium server.
type Options struct {
	Addr         string
	PluginsDir   string
	Plan         assembly.Plan
	Srv          *serve.Server
	CommandPlane CommandPlane
	ReplaySize   int
	// Layout is the merged layout (ADR-0012). Required for /api/layout.
	Layout any
	// UIMounts are Assembly-adjudicated mounts served on /api/plugins.
	UIMounts []assembly.EffectiveMount
	// DefaultWorkspace is applied to new Sessions when the client omits one (ADR-0020).
	DefaultWorkspace string
}

// CommandPlane is the slash-command surface the Shell uses (implemented by
// internal/app's webCommandPlane).
type CommandPlane interface {
	HandleOut(line string) (output string, quit bool, err error)
	Complete(prefix string) []string
}

// Server is the HTTP Web Render Medium.
type Server struct {
	opts Options
	http *http.Server

	mu      sync.Mutex
	hub     map[chan Event]struct{}
	replay  []Event
	replayN int

	// plugin name -> dir for /plugin-ui/
	uiDirs map[string]string

	// current default session for the Web Shell ("" = Host default).
	sessMu sync.Mutex
	sessID string
	// defaultWorkspace is applied to new Sessions when the client omits one.
	defaultWorkspace string
	// pending tool approvals (policy.ask → Render Medium).
	apprMu     sync.Mutex
	apprNext   int
	apprWait   map[string]chan bool
}

// currentSession resolves the Current Session from the session Capability
// (ADR-0012), falling back to the medium cache when the Capability is absent.
func (s *Server) currentSession() string {
	if s.opts.Srv != nil {
		out, err := s.opts.Srv.CallByCap("session", "current", json.RawMessage(`{}`))
		if err == nil && out != nil {
			var res struct {
				OK     bool `json:"ok"`
				Result struct {
					SessionID string `json:"sessionId"`
				} `json:"result"`
			}
			if json.Unmarshal(out, &res) == nil && res.OK {
				return res.Result.SessionID
			}
		}
	}
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	return s.sessID
}

func (s *Server) setCurrentSession(id string) {
	s.sessMu.Lock()
	s.sessID = id
	s.sessMu.Unlock()
	if s.opts.Srv != nil && id != "" {
		payload, _ := json.Marshal(map[string]string{"sessionId": id})
		_, _ = s.opts.Srv.CallByCap("session", "select", payload)
	}
}

// Event is one SSE payload.
type Event struct {
	Topic string `json:"-"`
	Data  any    `json:"-"`
}

type wireEvent struct {
	Topic string `json:"topic"`
	Data  any    `json:"data"`
}

// New builds a Web Medium server (does not listen yet).
func New(opts Options) *Server {
	if opts.ReplaySize <= 0 {
		opts.ReplaySize = DefaultReplay
	}
	s := &Server{
		opts:             opts,
		hub:              make(map[chan Event]struct{}),
		uiDirs:           map[string]string{},
		defaultWorkspace: opts.DefaultWorkspace,
		apprWait:         make(map[string]chan bool),
	}
	for _, p := range opts.Plan.Mounted {
		if p.Manifest.UI != nil && p.Manifest.UI.Entry != "" {
			s.uiDirs[p.Manifest.Name] = filepath.Join(p.Dir, "ui")
		}
	}
	// Fan-out from Host events.
	if opts.Srv != nil {
		opts.Srv.Subscribe(&serve.Subscriber{
			OnEvent: func(e serve.Event) {
				s.broadcast(Event{Topic: e.Topic, Data: e.Data})
			},
		})
		// policy.ask → Web Medium (ADR-0019): broadcast and wait for /api/tool-approval.
		opts.Srv.OnToolApproval = s.requestToolApproval
		// Seed replay with panels emitted during mount (before Subscribe).
		for _, p := range opts.Srv.Panels() {
			s.broadcast(Event{Topic: "panel", Data: p})
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/trace", s.handleTracePage)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/api/message", s.handleMessage)
	mux.HandleFunc("/api/command", s.handleCommand)
	mux.HandleFunc("/api/ui-action", s.handleUIAction)
	mux.HandleFunc("/api/call", s.handleCall)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/api/layout", s.handleLayout)
	mux.HandleFunc("/api/session/new", s.handleSessionNew)
	mux.HandleFunc("/api/session", s.handleSessionGet)
	mux.HandleFunc("/api/sessions", s.handleSessionsList)
	mux.HandleFunc("/api/session/select", s.handleSessionSelect)
	mux.HandleFunc("/api/session/workspace", s.handleSessionWorkspace)
	mux.HandleFunc("/api/tool-approval", s.handleToolApproval)
	mux.HandleFunc("/api/turn/cancel", s.handleTurnCancel)
	mux.HandleFunc("/plugin-ui/", s.handlePluginUI)
	mux.HandleFunc("/sdk/lite-agent.js", s.handleSDK)
	mux.Handle("/app/", http.StripPrefix("/app/", withNoCache(http.FileServer(http.FS(appStatic())))))
	s.http = &http.Server{Addr: opts.Addr, Handler: mux}
	return s
}

// Serve serves on an existing listener.
func (s *Server) Serve(ln net.Listener) error {
	return s.http.Serve(ln)
}

// ListenAndServe blocks.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Close shuts the HTTP server down.
func (s *Server) Close() error {
	return s.http.Close()
}

func (s *Server) broadcast(e Event) {
	s.mu.Lock()
	// Chronological ring: drop oldest when full.
	if len(s.replay) < s.opts.ReplaySize {
		s.replay = append(s.replay, e)
	} else {
		copy(s.replay, s.replay[1:])
		s.replay[len(s.replay)-1] = e
	}
	s.replayN++
	for ch := range s.hub {
		select {
		case ch <- e:
		default:
			// Slow consumer: sacrifice stream bursts; non-stream gets one async retry.
			if e.Topic == "stream" {
				continue
			}
			c, ev := ch, e
			go func() {
				select {
				case c <- ev:
				case <-time.After(2 * time.Second):
				}
			}()
		}
	}
	s.mu.Unlock()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(shellHTML))
}

func (s *Server) handleTracePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(traceHTML))
}

func (s *Server) handleSDK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write([]byte(sdkJS))
}

// appStatic serves the Shell's ES module tree from the embedded FS.
func appStatic() fs.FS {
	sub, err := fs.Sub(appFS, "static/app")
	if err != nil {
		panic(err) // embed layout is static; unreachable
	}
	return sub
}

// withNoCache keeps module files always-fresh: the Shell is embedded in the
// binary, so a cached stale module would survive host upgrades.
func withNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fl.Flush()

	ch := make(chan Event, 256)
	s.mu.Lock()
	s.hub[ch] = struct{}{}
	var backlog []Event
	if r.URL.Query().Get("replay") == "1" {
		backlog = append(backlog, s.replay...)
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.hub, ch)
		s.mu.Unlock()
	}()

	write := func(e Event) {
		data, err := json.Marshal(wireEvent{Topic: e.Topic, Data: e.Data})
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Topic, data)
		fl.Flush()
	}
	for _, e := range backlog {
		write(e)
	}
	notify := r.Context().Done()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-notify:
			return
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": keepalive\n\n")
			fl.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			write(e)
		}
	}
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		http.Error(w, "no agent server", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Text      string `json:"text"`
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	sid := in.SessionID
	if sid == "" {
		sid = s.currentSession()
	}
	// Parallel turns across Sessions; reject only when THIS Session is busy.
	if s.opts.Srv.IsRunningOn(sid) {
		writeJSON(w, map[string]any{
			"ok":               false,
			"error":            "this session already has a running turn",
			"runningSessionId": sid,
			"sessionId":        sid,
		})
		return
	}
	// Run turn asynchronously so the request returns; events stream on /events.
	go func() {
		_, err := s.opts.Srv.RunTurnOn(sid, in.Text)
		if err != nil {
			s.broadcast(Event{Topic: "status", Data: map[string]string{
				"status":    "error:" + err.Error(),
				"sessionId": sid,
			}})
		}
	}()
	writeJSON(w, map[string]any{"ok": true, "sessionId": sid})
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Line string `json:"line"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if s.opts.CommandPlane == nil {
		http.Error(w, "no command plane", http.StatusNotImplemented)
		return
	}
	out, quit, err := s.opts.CommandPlane.HandleOut(in.Line)
	writeJSON(w, map[string]any{"ok": err == nil, "quit": quit, "output": out, "error": errString(err)})
}

func (s *Server) handleSessionNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		http.Error(w, "no agent server", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Workspace string `json:"workspace"`
		SessionID string `json:"sessionId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	workspace := strings.TrimSpace(in.Workspace)
	if workspace == "" {
		workspace = s.defaultWorkspace
	}
	id, err := s.opts.Srv.NewSessionIDWithWorkspace(in.SessionID, workspace)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.setCurrentSession(id)
	s.broadcast(Event{Topic: "status", Data: map[string]string{"status": "session:" + id, "sessionId": id}})
	writeJSON(w, map[string]any{"ok": true, "sessionId": id, "status": "idle", "workspace": workspace})
}

func (s *Server) handleSessionWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		http.Error(w, "no agent server", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		SessionID string `json:"sessionId"`
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Workspace) == "" {
		http.Error(w, "workspace required", http.StatusBadRequest)
		return
	}
	sid := in.SessionID
	if sid == "" {
		sid = s.currentSession()
	}
	if err := s.opts.Srv.SetSessionWorkspace(sid, in.Workspace); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "sessionId": sid, "workspace": in.Workspace})
}

// requestToolApproval implements Host OnToolApproval for the Web Medium (ADR-0019).
// Broadcasts tool_approval on SSE; waits for /api/tool-approval. Timeout denies.
func (s *Server) requestToolApproval(tool string, arguments json.RawMessage, workspace, sessionID string) bool {
	s.apprMu.Lock()
	s.apprNext++
	id := fmt.Sprintf("appr-%d", s.apprNext)
	ch := make(chan bool, 1)
	s.apprWait[id] = ch
	s.apprMu.Unlock()
	defer func() {
		s.apprMu.Lock()
		delete(s.apprWait, id)
		s.apprMu.Unlock()
	}()
	s.broadcast(Event{Topic: "tool_approval", Data: map[string]any{
		"id":        id,
		"tool":      tool,
		"arguments": json.RawMessage(arguments),
		"workspace": workspace,
		"sessionId": sessionID,
	}})
	select {
	case ok := <-ch:
		return ok
	case <-time.After(2 * time.Minute):
		return false
	}
}

func (s *Server) handleToolApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		ID       string `json:"id"`
		Approved bool   `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ID == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	s.apprMu.Lock()
	ch, ok := s.apprWait[in.ID]
	s.apprMu.Unlock()
	if !ok {
		writeJSON(w, map[string]any{"ok": false, "error": "unknown approval id"})
		return
	}
	select {
	case ch <- in.Approved:
	default:
	}
	writeJSON(w, map[string]any{"ok": true, "approved": in.Approved})
}

func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	sid := s.currentSession()
	status := "idle"
	var running []string
	if s.opts.Srv != nil {
		status = s.opts.Srv.StatusForSession(sid)
		running = s.opts.Srv.RunningSessions()
	}
	writeJSON(w, map[string]any{
		"sessionId": sid,
		"status":    status,
		"running":   running,
		"busy":      len(running) > 0,
	})
}

func (s *Server) handleSessionsList(w http.ResponseWriter, r *http.Request) {
	if s.opts.Srv == nil {
		writeJSON(w, map[string]any{"sessions": []any{}, "current": s.currentSession()})
		return
	}
	list, err := s.opts.Srv.ListSessions()
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "sessions": []any{}, "current": s.currentSession()})
		return
	}
	writeJSON(w, map[string]any{"sessions": list, "current": s.currentSession()})
}

func (s *Server) handleSessionSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.SessionID == "" {
		http.Error(w, "sessionId required", http.StatusBadRequest)
		return
	}
	s.setCurrentSession(in.SessionID)
	s.broadcast(Event{Topic: "status", Data: map[string]string{"status": "session:" + in.SessionID, "sessionId": in.SessionID}})
	status := "idle"
	if s.opts.Srv != nil {
		status = s.opts.Srv.StatusForSession(in.SessionID)
	}
	writeJSON(w, map[string]any{"ok": true, "sessionId": in.SessionID, "status": status})
}

func (s *Server) handleTurnCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		http.Error(w, "no agent server", http.StatusServiceUnavailable)
		return
	}
	sid := s.currentSession()
	var in struct {
		SessionID string `json:"sessionId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.SessionID != "" {
		sid = in.SessionID
	}
	// Cancel only the target Session so parallel turns keep running.
	s.opts.Srv.CancelTurnOn(sid)
	s.broadcast(Event{Topic: "status", Data: map[string]string{"status": "cancelling", "sessionId": sid}})
	writeJSON(w, map[string]any{"ok": true, "sessionId": sid})
}

func (s *Server) handleUIAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "no agent server"})
		return
	}
	var in struct {
		Plugin string          `json:"plugin"`
		Panel  string          `json:"panel"`
		Event  string          `json:"event"`
		Value  json.RawMessage `json:"value"`
		Props  json.RawMessage `json:"props"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Plugin == "" {
		http.Error(w, "plugin required", http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"panel": in.Panel,
		"event": in.Event,
		"value": json.RawMessage(orEmptyJSON(in.Value)),
		"props": json.RawMessage(orEmptyJSON(in.Props)),
	})
	out, err := s.opts.Srv.CallUIAction(in.Plugin, payload)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": json.RawMessage(orEmptyJSON(out))})
}

// handleCall is LiteAgent.call: route cap.method to the plugin that provides cap.
// Optional "plugin" targets a named Plugin directly (config settings per plugin).
func (s *Server) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		writeJSON(w, map[string]any{"ok": false, "error": "no agent server"})
		return
	}
	var in struct {
		Cap     string          `json:"cap"`
		Method  string          `json:"method"`
		Payload json.RawMessage `json:"payload"`
		Plugin  string          `json:"plugin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Cap == "" || in.Method == "" {
		http.Error(w, "cap and method required", http.StatusBadRequest)
		return
	}
	if len(in.Payload) == 0 {
		in.Payload = json.RawMessage(`{}`)
	}
	var out json.RawMessage
	var err error
	if in.Plugin != "" {
		out, err = s.opts.Srv.CallByPlugin(in.Plugin, in.Cap, in.Method, in.Payload)
	} else {
		out, err = s.opts.Srv.CallByCap(in.Cap, in.Method, in.Payload)
	}
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": json.RawMessage(orEmptyJSON(out))})
}

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	if s.opts.Layout == nil {
		http.Error(w, "layout not loaded", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.opts.Layout)
}

// graphEdge is one directed dependency in the plugin graph.
// kind: provides | host-uses | consumes | ui-mount
type graphEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Kind       string `json:"kind"`
	Capability string `json:"capability,omitempty"`
	// UI mount extras
	Page      string `json:"page,omitempty"`
	Slot      string `json:"slot,omitempty"`
	Component string `json:"component,omitempty"`
}

// graphNode is one vertex: plugin, host, capability, or UI slot.
type graphNode struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"` // plugin | host | capability | slot
	Label       string   `json:"label"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Provides    []string `json:"provides,omitempty"`
	Consumes    []string `json:"consumes,omitempty"`
	// Unmet lists consumed capabilities no mounted plugin provides.
	Unmet []string `json:"unmet,omitempty"`
	// UI mount summary for plugin nodes
	Mounts []map[string]string `json:"mounts,omitempty"`
}

// hostUsedCapabilities are Capabilities the Host star-routes (Agent Loop / media).
var hostUsedCapabilities = []string{
	serve.SessionCap, serve.LLMCap, serve.ToolsCap,
	serve.SystemPromptCap, serve.ContextCap, serve.LoopCap,
}

// handlePlugins reports mounted plugins, Assembly-adjudicated UI mounts,
// and a relationship graph: plugin → capability → host, plus UI mounts.
func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	type uiItem struct {
		Entry  string                    `json:"entry"`
		Trust  string                    `json:"trust,omitempty"`
		Mounts []assembly.EffectiveMount `json:"mounts"`
		Pages  []plugin.UIPage           `json:"pages,omitempty"`
	}
	type item struct {
		Name        string               `json:"name"`
		Version     string               `json:"version,omitempty"`
		Description string               `json:"description,omitempty"`
		Provides    []string             `json:"provides,omitempty"`
		Consumes    []string             `json:"consumes,omitempty"`
		Commands    []plugin.CommandSpec `json:"commands,omitempty"`
		UI          *uiItem              `json:"ui,omitempty"`
	}

	providers := map[string][]string{}
	for _, p := range s.opts.Plan.Mounted {
		for _, c := range p.Manifest.Provides {
			providers[c] = append(providers[c], p.Manifest.Name)
		}
	}

	var list []item
	var nodes []graphNode
	var edges []graphEdge
	seenNode := map[string]bool{}
	seenEdge := map[string]bool{}

	addNode := func(n graphNode) {
		if seenNode[n.ID] {
			return
		}
		seenNode[n.ID] = true
		nodes = append(nodes, n)
	}
	addEdge := func(e graphEdge) {
		key := e.From + "\x00" + e.To + "\x00" + e.Kind + "\x00" + e.Capability + "\x00" + e.Component
		if seenEdge[key] {
			return
		}
		seenEdge[key] = true
		edges = append(edges, e)
	}

	// Host vertex: always present — the star-router that uses Capabilities.
	addNode(graphNode{ID: "host", Kind: "host", Label: "Host", Description: "Plugin host: assembly, routing, session invariant"})

	for _, p := range s.opts.Plan.Mounted {
		it := item{Name: p.Manifest.Name, Version: p.Manifest.Version,
			Description: p.Manifest.Description,
			Provides:    p.Manifest.Provides, Consumes: p.Manifest.Consumes,
			Commands: p.Manifest.Commands}
		node := graphNode{
			ID: p.Manifest.Name, Kind: "plugin", Label: p.Manifest.Name,
			Version: p.Manifest.Version, Description: p.Manifest.Description,
			Provides: p.Manifest.Provides, Consumes: p.Manifest.Consumes,
		}
		// provides: plugin → capability
		for _, capName := range p.Manifest.Provides {
			capID := "cap:" + capName
			addNode(graphNode{ID: capID, Kind: "capability", Label: capName})
			addEdge(graphEdge{From: p.Manifest.Name, To: capID, Kind: "provides", Capability: capName})
		}
		// consumes: capability → plugin (plugin depends on the capability)
		for _, need := range p.Manifest.Consumes {
			capID := "cap:" + need
			owners := providers[need]
			if len(owners) == 0 {
				node.Unmet = append(node.Unmet, need)
				addNode(graphNode{ID: capID, Kind: "capability", Label: need})
				addEdge(graphEdge{From: capID, To: p.Manifest.Name, Kind: "consumes", Capability: need})
				continue
			}
			addNode(graphNode{ID: capID, Kind: "capability", Label: need})
			addEdge(graphEdge{From: capID, To: p.Manifest.Name, Kind: "consumes", Capability: need})
		}
		// UI mounts: plugin → slot
		if ui := p.Manifest.UI; ui != nil && ui.Entry != "" {
			entry := strings.TrimPrefix(p.Manifest.UI.NormalizedEntry(), "ui/")
			var mounts []assembly.EffectiveMount
			if s.opts.UIMounts != nil {
				for _, m := range s.opts.UIMounts {
					if m.Plugin == p.Manifest.Name {
						mounts = append(mounts, m)
					}
				}
			} else {
				for _, m := range ui.Mounts {
					page := m.Page
					if page == "" {
						page = "main"
					}
					mounts = append(mounts, assembly.EffectiveMount{
						Plugin: p.Manifest.Name, Page: page, Slot: m.Slot,
						Component: m.Component, Props: m.Props,
					})
				}
			}
			for _, m := range mounts {
				slotID := "ui:" + m.Page + "/" + m.Slot
				addNode(graphNode{ID: slotID, Kind: "slot", Label: m.Page + " · " + m.Slot})
				addEdge(graphEdge{
					From: p.Manifest.Name, To: slotID, Kind: "ui-mount",
					Page: m.Page, Slot: m.Slot, Component: m.Component,
				})
				node.Mounts = append(node.Mounts, map[string]string{
					"page": m.Page, "slot": m.Slot, "component": m.Component,
				})
			}
			it.UI = &uiItem{
				Entry:  "/plugin-ui/" + p.Manifest.Name + "/" + entry,
				Trust:  ui.Trust,
				Mounts: mounts,
				Pages:  ui.Pages,
			}
		}
		addNode(node)
		list = append(list, it)
	}

	// Host uses each Capability that some mounted plugin provides.
	for _, capName := range hostUsedCapabilities {
		if len(providers[capName]) == 0 {
			continue
		}
		capID := "cap:" + capName
		addNode(graphNode{ID: capID, Kind: "capability", Label: capName})
		addEdge(graphEdge{From: capID, To: "host", Kind: "host-uses", Capability: capName})
	}

	if nodes == nil {
		nodes = []graphNode{}
	}
	if edges == nil {
		edges = []graphEdge{}
	}
	writeJSON(w, map[string]any{
		"plugins": list,
		"graph": map[string]any{
			"nodes": nodes,
			"edges": edges,
		},
	})
}

func (s *Server) handlePluginUI(w http.ResponseWriter, r *http.Request) {
	// /plugin-ui/<name>/<rel>
	rest := strings.TrimPrefix(r.URL.Path, "/plugin-ui/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	root, ok := s.uiDirs[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	rel := "main.js" // UI Entry default (ADR-0010)
	if len(parts) == 2 {
		rel = parts[1]
	}
	// Path cleaning: never escape the plugin ui dir.
	clean := path.Clean("/" + rel)
	full := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	absRoot, _ := filepath.Abs(root)
	absFull, _ := filepath.Abs(full)
	if absFull != absRoot && !strings.HasPrefix(absFull, absRoot+string(os.PathSeparator)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, absFull)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func orEmptyJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
