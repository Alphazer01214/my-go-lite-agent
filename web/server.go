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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
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
	// pending tool approvals (policy.ask 鈫?Render Medium).
	apprMu   sync.Mutex
	apprNext int
	apprWait map[string]chan bool
}

// currentSession resolves the Current Session from the session plugin
// (point-named, ADR-0030), falling back to the medium cache when absent.
func (s *Server) currentSession() string {
	if s.opts.Srv != nil {
		out, err := s.opts.Srv.CallByPlugin("session", "session", "current", json.RawMessage(`{}`))
		if err == nil && out != nil {
			var res struct {
				SessionID string `json:"sessionId"`
			}
			if json.Unmarshal(out, &res) == nil && res.SessionID != "" {
				return res.SessionID
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
		_, _ = s.opts.Srv.CallByPlugin("session", "session", "select", payload)
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
		// policy.ask 鈫?Web Medium (ADR-0019/0029): register the approval face;
		// CLI and Web can both be registered without overwriting each other.
		serve.RegisterApproval(opts.Srv, s.requestToolApproval)
		// Seed replay with panels emitted during mount (before Subscribe).
		for _, p := range opts.Srv.Panels() {
			s.broadcast(Event{Topic: "panel", Data: p})
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
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
	mux.HandleFunc("/api/workspace/resolve", s.handleWorkspaceResolve)
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
	// Run turn asynchronously so the request returns; events stream on /events.
	// Busy/lock enforcement lives in the loop plugin (ADR-0030).
	go func() {
		payload, _ := json.Marshal(map[string]any{"input": in.Text, "sessionId": sid, "allowSubagent": true})
		_, err := s.opts.Srv.CallByPlugin("agent", "loop", "turn", payload)
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
	id := in.SessionID
	if id == "" {
		id = fmt.Sprintf("s-%d", time.Now().UnixNano())
	}
	// session.create is the public Session contract (ADR-0020); origin=web
	// so subagent-created sessions never steal Current (CONTEXT.md).
	payload, _ := json.Marshal(map[string]any{
		"sessionId": id, "origin": "web", "workspace": workspace,
	})
	if _, err := s.opts.Srv.CallByPlugin("session", "session", "create", payload); err != nil {
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
	// Session metadata including Workspace is read via the public session.info
	// contract; setting Workspace upserts via session.create (ADR-0020).
	payload, _ := json.Marshal(map[string]any{"sessionId": sid, "workspace": in.Workspace})
	if _, err := s.opts.Srv.CallByPlugin("session", "session", "create", payload); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "sessionId": sid, "workspace": in.Workspace})
}

// workspaceResolveMaxDirs caps the directory walk so a pick never hangs the Host.
const workspaceResolveMaxDirs = 20000

// skipWalkDirs are directory names never searched for a picked workspace.
var skipWalkDirs = map[string]bool{
	"node_modules": true, "AppData": true, "$RECYCLE.BIN": true,
	"System Volume Information": true, "Windows": true, "ProgramData": true,
}

// handleWorkspaceResolve maps a folder chosen in the browser to an absolute path.
//
// The File System Access API (showDirectoryPicker) deliberately never reveals a
// path 鈥?only the directory NAME. The Web Workspace (ADR-0020) needs a real
// path. The directory walk lives in the workspace Capability provider (a
// Plugin, ADR-0026: Host does not traverse the filesystem for plugin business);
// this handler only relays name 鈫?{path, candidates}.
func (s *Server) handleWorkspaceResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.opts.Srv == nil {
		http.Error(w, "no agent server", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(map[string]string{"name": strings.TrimSpace(in.Name)})
	// workspace.resolve was a Host L1 face; under ADR-0030 no provider is
	// guaranteed — fall back to empty so the picker still works.
	out, err := s.opts.Srv.CallByPlugin("filetools", "workspace", "resolve", payload)
	if err != nil {
		// No provider: fall back to "type the path" (empty result), do not fail
		// the whole picker.
		writeJSON(w, map[string]any{"ok": true, "name": in.Name, "path": "", "candidates": []string{}})
		return
	}
	var res struct {
		Name       string   `json:"name"`
		Path       string   `json:"path"`
		Candidates []string `json:"candidates"`
	}
	if len(out) > 0 {
		_ = json.Unmarshal(out, &res)
	}
	if res.Name == "" {
		res.Name = in.Name
	}
	if res.Candidates == nil {
		res.Candidates = []string{}
	}
	writeJSON(w, map[string]any{"ok": true, "name": res.Name, "path": res.Path, "candidates": res.Candidates})
}

// requestToolApproval implements Host OnToolApproval for the Web Medium (ADR-0019).
// Broadcasts tool_approval on SSE; waits for /api/tool-approval. Timeout denies.
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
		status = "idle"
		running = nil
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
	out, err := s.opts.Srv.CallByPlugin("session", "session", "list", json.RawMessage(`{}`))
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "sessions": []any{}, "current": s.currentSession()})
		return
	}
	var listRes struct {
		Sessions []map[string]any `json:"sessions"`
	}
	if len(out) > 0 {
		_ = json.Unmarshal(out, &listRes)
	}
	if listRes.Sessions == nil {
		listRes.Sessions = []map[string]any{}
	}
	writeJSON(w, map[string]any{"sessions": listRes.Sessions, "current": s.currentSession()})
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
		status = "idle"
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
	cancelLoop(s.opts.Srv, sid)
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
	// UI Actions address the plugin by name through its declared ui hostFace
	// (ADR-0027): Host validates the face declaration, no registry bypass.
	out, err := serve.CallByFace(s.opts.Srv, in.Plugin, "ui", "action", payload)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": json.RawMessage(orEmptyJSON(out))})
}

// handleCall is LiteAgent.call: point-named plugin invocation (ADR-0030).
// Body: {to, cap, method, payload} or legacy {plugin, cap, method} for hostFaces.
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
		To      string          `json:"to"`
		Cap     string          `json:"cap"`
		Method  string          `json:"method"`
		Payload json.RawMessage `json:"payload"`
		Plugin  string          `json:"plugin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	target := in.To
	if target == "" {
		target = in.Plugin
	}
	if target == "" || in.Method == "" {
		http.Error(w, "to and method required", http.StatusBadRequest)
		return
	}
	if len(in.Payload) == 0 {
		in.Payload = json.RawMessage(`{}`)
	}
	capName := in.Cap
	if capName == "" {
		capName = target
	}
	var out json.RawMessage
	var err error
	// hostFaces (config|commands|ui) go through CallByFace validation.
	if capName == "config" || capName == "commands" || capName == "ui" {
		out, err = serve.CallByFace(s.opts.Srv, target, capName, in.Method, in.Payload)
	} else {
		out, err = s.opts.Srv.CallByPlugin(target, capName, in.Method, in.Payload)
	}
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": json.RawMessage(orEmptyJSON(out))})
}

func cancelLoop(srv *serve.Server, sessionID string) {
	if srv == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"sessionId": sessionID})
	_, _ = srv.CallByPlugin("agent", "loop", "cancel", payload)
}

func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	if s.opts.Layout == nil {
		http.Error(w, "layout not loaded", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.opts.Layout)
}

// Node/edge mount states for the plugin graph.
const (
	// StateMounted is a Plugin whose process/UI is live in the Host right now.
	StateMounted = "mounted"
	// StateAvailable is a discovered Plugin that is not mounted yet: it can be
	// pulled in later by a dependsOn closure or an Agent Scheme dependsPlugins.
	StateAvailable = "available"
	// StateDegraded is a mounted Plugin whose consumes are unmet (ADR-0022).
	StateDegraded = "degraded"
	// StateMissing is a name referenced by dependsOn that Discovery never saw.
	StateMissing = "missing"
)

// graphEdge is one directed dependency in the plugin graph.
// kind: provides | host-uses | consumes | ui-mount | depends-on | scheme
type graphEdge struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Kind       string `json:"kind"`
	Capability string `json:"capability,omitempty"`
	// Scheme is the Agent Scheme that pulls To (kind=scheme).
	Scheme string `json:"scheme,omitempty"`
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
	// State is the mount state (mounted|available|degraded|missing).
	State string `json:"state,omitempty"`
	// Autostart marks a Host startup root (ADR-0021).
	Autostart bool `json:"autostart,omitempty"`
	// HostFaces lists the Host-addressed faces this plugin serves (ADR-0027).
	HostFaces []string `json:"hostFaces,omitempty"`
	// DependsOn lists plugin names this node pulls in.
	DependsOn []string `json:"dependsOn,omitempty"`
	// Schemes lists Agent Schemes whose dependsPlugins include this plugin.
	Schemes []string `json:"schemes,omitempty"`
	// Tools are the tool names this plugin owns (tools Capability providers).
	Tools []string `json:"tools,omitempty"`
	// Unmet lists consumed capabilities no mounted plugin provides.
	Unmet []string `json:"unmet,omitempty"`
	// UI mount summary for plugin nodes
	Mounts []map[string]string `json:"mounts,omitempty"`
}

// agentSchemeFace is the slice of the agent plugin's config face the graph needs.
type agentSchemeFace struct {
	DefaultScheme string `json:"defaultScheme"`
	Schemes       map[string]struct {
		DependsPlugins []string `json:"dependsPlugins,omitempty"`
	} `json:"schemes"`
}

// Host no longer star-routes capabilities (ADR-0030). Graph host-uses edges
// stay empty; provides/consumes remain declaration data only.
var hostUsedCapabilities = []string{}

// handlePlugins reports the plugin catalog and its relationship graph.
//
// The graph is drawn from the whole Discovery catalog, not just the live set:
// under Autostart+dependsOn (ADR-0021) most plugins mount lazily (Agent Scheme
// 鈫?ensurePlugins, ADR-0023), so a live-only graph would stay half-empty until
// the first Turn. Every plugin node carries its state (mounted|available|
// degraded|missing) and the Agent Scheme edges that will pull it in.
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
		DependsOn   []string             `json:"dependsOn,omitempty"`
		Autostart   bool                 `json:"autostart,omitempty"`
		State       string               `json:"state"`
		HostFaces   []string             `json:"hostFaces,omitempty"`
		Tools       []string             `json:"tools,omitempty"`
		Schemes     []string             `json:"schemes,omitempty"`
		Commands    []plugin.CommandSpec `json:"commands,omitempty"`
		Degraded    bool                 `json:"degraded,omitempty"`
		UI          *uiItem              `json:"ui,omitempty"`
	}

	live := map[string]bool{}
	degraded := map[string]bool{}
	reconcileGen := 0
	registryProvides := map[string]string{}
	if s.opts.Srv != nil {
		reg := serve.Registry(s.opts.Srv)
		reconcileGen = reg.ReconcileGen
		degraded = map[string]bool{}
		for _, n := range reg.Degraded {
			degraded[n] = true
		}
		registryProvides = reg.Provides
		for _, n := range s.opts.Srv.MountedPluginNames() {
			live[n] = true
		}
	}

	// Catalog = mount plan 鈭?everything Discovery sees under -plugins.
	byName := map[string]discovery.Found{}
	var order []string
	addFound := func(p discovery.Found) {
		name := p.Manifest.Name
		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = p
	}
	for _, p := range s.opts.Plan.Mounted {
		addFound(p)
		live[p.Manifest.Name] = true
	}
	if s.opts.PluginsDir != "" {
		if res := discovery.Scan(s.opts.PluginsDir); len(res.Plugins) > 0 {
			for _, p := range res.Plugins {
				addFound(p)
			}
		}
	}

	// Agent Scheme face: which plugins each scheme pulls in (ADR-0023).
	// Read through the agent-presets Capability (ADR-0027): Host never reaches
	// into agent config; when no Agent Plugin provides it, no scheme edges.
	schemeName := ""
	schemeNames := []string{}
	schemePulls := map[string][]string{} // scheme -> plugin names
	if s.opts.Srv != nil {
		if out, err := s.opts.Srv.CallByPlugin("agent", "agent-presets", "get", json.RawMessage(`{}`)); err == nil && len(out) > 0 {
			var face agentSchemeFace
			_ = json.Unmarshal(out, &face)
			schemeName = face.DefaultScheme
			for name, sc := range face.Schemes {
				schemeNames = append(schemeNames, name)
				schemePulls[name] = sc.DependsPlugins
			}
		}
	}
	sort.Strings(schemeNames)
	pulledBy := map[string][]string{} // plugin -> schemes that pull it
	for _, name := range schemeNames {
		for _, p := range schemePulls[name] {
			pulledBy[p] = append(pulledBy[p], name)
		}
	}

	stateOf := func(name string) string {
		if degraded[name] {
			return StateDegraded
		}
		if live[name] {
			return StateMounted
		}
		return StateAvailable
	}
	// Catalog order: mounted first, then degraded, then available; stable by name.
	rank := map[string]int{StateMounted: 0, StateDegraded: 1, StateAvailable: 2}
	sort.SliceStable(order, func(i, j int) bool {
		ri, rj := rank[stateOf(order[i])], rank[stateOf(order[j])]
		if ri != rj {
			return ri < rj
		}
		return order[i] < order[j]
	})

	// Capability owners come from the actual Host registry (ADR-0027): what
	// routes today. Degraded plugins are excluded so "unmet" stays honest.
	providers := map[string][]string{}
	for cap, owner := range registryProvides {
		if degraded[owner] {
			continue
		}
		providers[cap] = append(providers[cap], owner)
	}
	// Declared capability owners come from the manifest of every catalog plugin;
	// a capability declared but with no live registry owner is the "declared vs
	// actual" delta the graph exposes through node state (ADR-0025).
	declared := map[string][]string{}
	for _, name := range order {
		for _, c := range byName[name].Manifest.Provides {
			declared[c] = append(declared[c], name)
		}
	}

	var list []item
	var nodes []graphNode
	var edges []graphEdge
	seenNode := map[string]bool{}
	seenEdge := map[string]bool{}
	missing := []string{}

	addNode := func(n graphNode) {
		if seenNode[n.ID] {
			return
		}
		seenNode[n.ID] = true
		nodes = append(nodes, n)
	}
	addEdge := func(e graphEdge) {
		key := e.From + "\x00" + e.To + "\x00" + e.Kind + "\x00" + e.Capability + "\x00" + e.Component + "\x00" + e.Scheme
		if seenEdge[key] {
			return
		}
		seenEdge[key] = true
		edges = append(edges, e)
	}
	// ensurePluginNode registers a dependency target that Discovery never saw.
	ensurePluginNode := func(name string) {
		if _, ok := byName[name]; ok {
			return
		}
		if seenNode[name] {
			return
		}
		missing = append(missing, name)
		addNode(graphNode{ID: name, Kind: "plugin", Label: name, State: StateMissing,
			Description: "referenced by dependsOn but not discovered"})
		seenNode[name] = true
	}

	// Host vertex: always present 鈥?the star-router that uses Capabilities.
	addNode(graphNode{ID: "host", Kind: "host", Label: "Host", State: StateMounted,
		Description: "Plugin host: discovery, routing, session invariant, ensurePlugins"})

	for _, name := range order {
		p := byName[name]
		st := stateOf(name)
		var tools []string
		it := item{
			Name: p.Manifest.Name, Version: p.Manifest.Version,
			Description: p.Manifest.Description,
			Provides:    p.Manifest.Provides, Consumes: p.Manifest.Consumes,
			DependsOn: p.Manifest.DependsOn, Autostart: p.Manifest.Autostart,
			State: st, HostFaces: p.Manifest.HostFaces, Tools: tools, Schemes: pulledBy[name],
			Commands: p.Manifest.Commands, Degraded: degraded[p.Manifest.Name],
		}
		node := graphNode{
			ID: p.Manifest.Name, Kind: "plugin", Label: p.Manifest.Name,
			Version: p.Manifest.Version, Description: p.Manifest.Description,
			Provides: p.Manifest.Provides, Consumes: p.Manifest.Consumes,
			State: st, Autostart: p.Manifest.Autostart, HostFaces: p.Manifest.HostFaces,
			DependsOn: p.Manifest.DependsOn,
			Schemes:   pulledBy[name], Tools: tools,
		}
		// provides: plugin 鈫?capability
		for _, capName := range p.Manifest.Provides {
			capID := "cap:" + capName
			capState := StateAvailable
			for _, owner := range providers[capName] {
				if live[owner] && !degraded[owner] {
					capState = StateMounted
					break
				}
			}
			addNode(graphNode{ID: capID, Kind: "capability", Label: capName, State: capState})
			addEdge(graphEdge{From: p.Manifest.Name, To: capID, Kind: "provides", Capability: capName})
		}
		// consumes: capability 鈫?plugin (plugin depends on the capability)
		for _, need := range p.Manifest.Consumes {
			capID := "cap:" + need
			addNode(graphNode{ID: capID, Kind: "capability", Label: need, State: StateAvailable})
			addEdge(graphEdge{From: capID, To: p.Manifest.Name, Kind: "consumes", Capability: need})
			if len(declared[need]) == 0 {
				node.Unmet = append(node.Unmet, need)
			}
		}
		// hostFaces: plugin 鈫?face (Host addresses the plugin by name, ADR-0027)
		for _, face := range p.Manifest.HostFaces {
			faceID := "face:" + face
			addNode(graphNode{ID: faceID, Kind: "face", Label: face, State: st})
			addEdge(graphEdge{From: p.Manifest.Name, To: faceID, Kind: "hostface", Capability: face})
		}
		// dependsOn: plugin 鈫?plugin (hard pull closure)
		for _, dep := range p.Manifest.DependsOn {
			ensurePluginNode(dep)
			addEdge(graphEdge{From: p.Manifest.Name, To: dep, Kind: "depends-on"})
		}
		// UI mounts: plugin 鈫?slot
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
				addNode(graphNode{ID: slotID, Kind: "slot", Label: m.Page + " 路 " + m.Slot, State: StateMounted})
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

	// Agent Scheme edges: which plugin each scheme will ensure (ADR-0023).
	// The agent vertex is the plugin that actually provides agent-presets
	// (resolved from the registry), never a hard-coded plugin name.
	agentOwner := ""
	for cap, owner := range registryProvides {
		if cap == "agent-presets" {
			agentOwner = owner
			break
		}
	}
	if agentOwner == "" {
		agentOwner = "agent-presets"
	}
	for _, sch := range schemeNames {
		for _, dep := range schemePulls[sch] {
			ensurePluginNode(dep)
			addEdge(graphEdge{From: agentOwner, To: dep, Kind: "scheme", Scheme: sch})
		}
	}

	// Host uses each Capability that some catalog plugin declares or provides.
	for _, capName := range hostUsedCapabilities {
		if len(declared[capName]) == 0 {
			continue
		}
		capID := "cap:" + capName
		capState := StateAvailable
		for _, owner := range providers[capName] {
			if live[owner] && !degraded[owner] {
				capState = StateMounted
				break
			}
		}
		addNode(graphNode{ID: capID, Kind: "capability", Label: capName, State: capState})
		addEdge(graphEdge{From: capID, To: "host", Kind: "host-uses", Capability: capName})
	}

	if nodes == nil {
		nodes = []graphNode{}
	}
	if edges == nil {
		edges = []graphEdge{}
	}
	if missing == nil {
		missing = []string{}
	}
	mountedNames := []string{}
	for _, name := range order {
		if stateOf(name) == StateMounted {
			mountedNames = append(mountedNames, name)
		}
	}
	writeJSON(w, map[string]any{
		"plugins": list,
		"graph": map[string]any{
			"nodes": nodes,
			"edges": edges,
		},
		"scheme":  schemeName,
		"schemes": schemeNames,
		"schemePulls": func() map[string][]string {
			out := map[string][]string{}
			for _, n := range schemeNames {
				out[n] = schemePulls[n]
				if out[n] == nil {
					out[n] = []string{}
				}
			}
			return out
		}(),
		"summary": map[string]any{
			"discovered":   len(order),
			"mounted":      len(mountedNames),
			"mountedNames": mountedNames,
			"degraded":     len(degraded),
			"missing":      missing,
			"reconcileGen": reconcileGen,
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
