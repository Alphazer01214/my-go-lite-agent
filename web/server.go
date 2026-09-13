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
}

// CommandPlane is the slash-command surface the Shell uses (implemented by cmd/host).
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
}

func (s *Server) currentSession() string {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	return s.sessID
}

func (s *Server) setCurrentSession(id string) {
	s.sessMu.Lock()
	s.sessID = id
	s.sessMu.Unlock()
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
		opts:   opts,
		hub:    make(map[chan Event]struct{}),
		uiDirs: map[string]string{},
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
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/trace", s.handleTrace)
	mux.HandleFunc("/api/session/new", s.handleSessionNew)
	mux.HandleFunc("/api/session", s.handleSessionGet)
	mux.HandleFunc("/api/sessions", s.handleSessionsList)
	mux.HandleFunc("/api/session/select", s.handleSessionSelect)
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

// handleHistory rehydrates the chat from Session Log facts (survives refresh).
// Includes reasoning/tool process rows — not just derived user/assistant.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.opts.Srv == nil {
		writeJSON(w, map[string]any{"facts": []any{}, "messages": []any{}})
		return
	}
	sid := r.URL.Query().Get("sessionId")
	if sid == "" {
		sid = s.currentSession()
	}
	facts, err := s.opts.Srv.QuerySessionFacts(sid, 0, 0)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "facts": []any{}, "messages": []any{}})
		return
	}
	if facts == nil {
		facts = []map[string]any{}
	}
	// Also keep derive-style messages for callers that only want the transcript.
	msgs, derr := s.opts.Srv.DeriveMessages(sid)
	type item struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	list := make([]item, 0)
	if derr == nil {
		for _, m := range msgs {
			if m.Role == "system" || m.Content == "" {
				continue
			}
			list = append(list, item{Role: m.Role, Content: m.Content})
		}
	}
	writeJSON(w, map[string]any{"facts": facts, "messages": list, "sessionId": sid})
}

// handleTrace returns Session Log facts as a turn/step trajectory for the sidebar.
func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	if s.opts.Srv == nil {
		writeJSON(w, map[string]any{"facts": []any{}})
		return
	}
	sid := r.URL.Query().Get("sessionId")
	if sid == "" {
		sid = s.currentSession()
	}
	facts, err := s.opts.Srv.QuerySessionFacts(sid, 0, 0)
	if err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "facts": []any{}})
		return
	}
	writeJSON(w, map[string]any{"facts": facts, "sessionId": sid})
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
	id, err := s.opts.Srv.NewSessionID("")
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.setCurrentSession(id)
	s.broadcast(Event{Topic: "status", Data: map[string]string{"status": "session:" + id, "sessionId": id}})
	writeJSON(w, map[string]any{"ok": true, "sessionId": id, "status": "idle"})
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
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Cap == "" || in.Method == "" {
		http.Error(w, "cap and method required", http.StatusBadRequest)
		return
	}
	out, err := s.opts.Srv.CallByCap(in.Cap, in.Method, in.Payload)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "result": json.RawMessage(orEmptyJSON(out))})
}

// handlePlugins reports mounted plugins plus, for UI plugins, the Panel
// Component contract (ADR-0010): entry URL and static mounts. The Shell
// imports each entry module and applies the mounts itself.
func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	type uiItem struct {
		Entry  string           `json:"entry"`
		Mounts []plugin.UIMount `json:"mounts"`
	}
	type item struct {
		Name     string               `json:"name"`
		Version  string               `json:"version,omitempty"`
		Provides []string             `json:"provides,omitempty"`
		Commands []plugin.CommandSpec `json:"commands,omitempty"`
		UI       *uiItem              `json:"ui,omitempty"`
	}
	var list []item
	for _, p := range s.opts.Plan.Mounted {
		it := item{Name: p.Manifest.Name, Version: p.Manifest.Version,
			Provides: p.Manifest.Provides, Commands: p.Manifest.Commands}
		if ui := p.Manifest.UI; ui != nil && ui.Entry != "" {
			entry := strings.TrimPrefix(p.Manifest.UI.NormalizedEntry(), "ui/")
			mounts := make([]plugin.UIMount, len(ui.Mounts))
			copy(mounts, ui.Mounts)
			it.UI = &uiItem{
				Entry:  "/plugin-ui/" + p.Manifest.Name + "/" + entry,
				Mounts: mounts,
			}
		}
		list = append(list, it)
	}
	writeJSON(w, map[string]any{"plugins": list})
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
