// Package web is the Host-embedded Web Render Medium (ADR-0009).
package web

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

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
	Handle(line string) (quit bool, err error)
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
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/api/message", s.handleMessage)
	mux.HandleFunc("/api/command", s.handleCommand)
	mux.HandleFunc("/api/ui-action", s.handleUIAction)
	mux.HandleFunc("/api/call", s.handleCall)
	mux.HandleFunc("/api/plugins", s.handlePlugins)
	mux.HandleFunc("/plugin-ui/", s.handlePluginUI)
	mux.HandleFunc("/sdk/lite-agent.js", s.handleSDK)
	s.http = &http.Server{Addr: opts.Addr, Handler: mux}
	s.injectManifestUIs()
	return s
}

// Serve serves on an existing listener.
func (s *Server) Serve(ln net.Listener) error {
	return s.http.Serve(ln)
}

// injectManifestUIs loads ui.entry HTML and emits mount-time panel set ops into the replay buffer.
func (s *Server) injectManifestUIs() {
	for _, p := range s.opts.Plan.Mounted {
		ui := p.Manifest.UI
		if ui == nil || ui.Entry == "" {
			continue
		}
		slot := "sidebar"
		if len(ui.Slots) > 0 && ui.Slots[0] != "" {
			slot = ui.Slots[0]
		}
		full := filepath.Join(p.Dir, ui.Entry)
		if !strings.HasPrefix(filepath.ToSlash(ui.Entry), "ui/") {
			full = filepath.Join(p.Dir, "ui", ui.Entry)
		}
		raw, err := os.ReadFile(full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: plugin %s ui.entry unreadable: %v\n", p.Manifest.Name, err)
			continue
		}
		s.broadcast(Event{Topic: "panel", Data: serve.PanelOp{
			Op:   "set",
			Slot: slot,
			ID:   p.Manifest.Name,
			HTML: string(raw),
		}})
	}
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
			// Slow consumer: drop rather than block the Agent Loop.
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
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(sdkJS))
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

	ch := make(chan Event, 64)
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
	for {
		select {
		case <-notify:
			return
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
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Text) == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	// Run turn asynchronously so the request returns; events stream on /events.
	go func() {
		_, err := s.opts.Srv.RunTurn(in.Text)
		if err != nil {
			s.broadcast(Event{Topic: "status", Data: map[string]string{"status": "error:" + err.Error()}})
		}
	}()
	writeJSON(w, map[string]any{"ok": true})
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
	quit, err := s.opts.CommandPlane.Handle(in.Line)
	writeJSON(w, map[string]any{"ok": err == nil, "quit": quit, "error": errString(err)})
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

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Name    string               `json:"name"`
		Slots   []string             `json:"slots,omitempty"`
		HasUI   bool                 `json:"hasUI"`
		Command []plugin.CommandSpec `json:"commands,omitempty"`
	}
	var list []item
	for _, p := range s.opts.Plan.Mounted {
		it := item{Name: p.Manifest.Name, Command: p.Manifest.Commands}
		if p.Manifest.UI != nil {
			it.HasUI = true
			it.Slots = p.Manifest.UI.Slots
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
	rel := "index.html"
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
