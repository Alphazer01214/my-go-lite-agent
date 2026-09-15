// Command session is an append-only Session Log Plugin (file-backed JSONL + memory).
//
// Capability: session
//   - create / append / query / list / derive / current / select
//
// Persistence: each Session is one JSONL file under the data directory
// (env SESSION_DATA_DIR, or ./sessions). Restart reloads facts.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const defaultSessionID = "default"

// defaultFullToolResults keeps this many newest tool_result contents full in derive.
const defaultFullToolResults = 8

type pluginConfig struct {
	FullToolResults int `json:"fullToolResults"`
}

func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func loadPluginConfig() pluginConfig {
	cfg := pluginConfig{FullToolResults: defaultFullToolResults}
	raw, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	var disk pluginConfig
	if err := json.Unmarshal(raw, &disk); err != nil {
		return cfg
	}
	if disk.FullToolResults > 0 {
		cfg.FullToolResults = disk.FullToolResults
	}
	return cfg
}

// stubOldToolResults replaces tool_result contents beyond the newest keep count
// with a short recoverable stub (ADR-0015). Session Log facts are unchanged;
// the input slice is copied so callers never observe in-place mutation.
func stubOldToolResults(msgs []Message, keep int) []Message {
	if keep <= 0 {
		keep = defaultFullToolResults
	}
	var idxs []int
	for i, m := range msgs {
		if m.Role == "tool" {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) <= keep {
		return msgs
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	for _, i := range idxs[:len(idxs)-keep] {
		n := len(out[i].Content)
		out[i].Content = fmt.Sprintf(
			"[truncated tool result: %d chars. Re-call the tool or read_file with offset/limit to recover.]",
			n,
		)
	}
	return out
}

// sessionItem is one row of the session list (Capability and slash command).
// Parent/origin make the Subagent tree visible so a parent Session can enter a child.
type sessionItem struct {
	ID              string `json:"id"`
	Title           string `json:"title,omitempty"`
	Seq             int    `json:"seq,omitempty"`
	ParentSession   string `json:"parentSession,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegationDepth,omitempty"`
}

type Fact struct {
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"meta,omitempty"`
	// Ts is append time (UnixMilli). Loaded facts keep their original ts;
	// facts written before this field existed have ts=0.
	Ts int64 `json:"ts,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type SessionMeta struct {
	CreatedAt       int64  `json:"createdAt"`
	ParentSession   string `json:"parentSession,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegationDepth,omitempty"`
}

type store struct {
	mu    sync.Mutex
	facts []Fact
	// path is the JSONL file; empty disables persistence.
	path string
	file *os.File
}

type registry struct {
	mu       sync.Mutex
	sessions map[string]*store
	meta     map[string]SessionMeta
	dataDir  string
}

func dataDir() string {
	if v := os.Getenv("SESSION_DATA_DIR"); v != "" {
		return v
	}
	return "sessions"
}

func newRegistry(dir string) *registry {
	if dir == "" {
		dir = dataDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	r := &registry{
		sessions: make(map[string]*store),
		meta:     make(map[string]SessionMeta),
		dataDir:  dir,
	}
	// Restore default session if present.
	r.openStore(defaultSessionID)
	return r
}

func normalizeID(id string) string {
	if id == "" {
		return defaultSessionID
	}
	return id
}

func safeID(id string) string {
	// Keep filenames boring.
	b := make([]rune, 0, len(id))
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b = append(b, r)
		} else {
			b = append(b, '_')
		}
	}
	if len(b) == 0 {
		return "session"
	}
	return string(b)
}

func (r *registry) openStore(id string) *store {
	id = normalizeID(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.sessions[id]; ok {
		return st
	}
	st := &store{path: filepath.Join(r.dataDir, safeID(id)+".jsonl")}
	st.load()
	if f, err := os.OpenFile(st.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		st.file = f
	}
	r.sessions[id] = st
	if _, ok := r.meta[id]; !ok {
		r.meta[id] = SessionMeta{CreatedAt: time.Now().UnixMilli()}
	}
	// Restart: recover parent/origin/depth from the session_meta fact when in-memory meta is bare.
	if m := r.meta[id]; m.ParentSession == "" && m.Origin == "" {
		if pm, ok := metaFromFacts(st.facts); ok && (pm.ParentSession != "" || pm.Origin != "") {
			if m.CreatedAt == 0 {
				m.CreatedAt = pm.CreatedAt
			}
			m.ParentSession = pm.ParentSession
			m.Origin = pm.Origin
			m.DelegationDepth = pm.DelegationDepth
			r.meta[id] = m
		}
	}
	return st
}

// metaFromFacts recovers SessionMeta from a session_meta fact (restart-safe parent links).
func metaFromFacts(facts []Fact) (SessionMeta, bool) {
	for i := len(facts) - 1; i >= 0; i-- {
		f := facts[i]
		if f.Type != "session_meta" || len(f.Meta) == 0 {
			continue
		}
		var m SessionMeta
		if err := json.Unmarshal(f.Meta, &m); err != nil {
			continue
		}
		return m, true
	}
	return SessionMeta{}, false
}

func (r *registry) create(id string, m SessionMeta) (SessionMeta, bool, error) {
	id = normalizeID(id)
	r.mu.Lock()
	if ex, ok := r.meta[id]; ok {
		// Upsert parent/origin when the caller supplies linkage (Subagent spawn
		// must not be a no-op just because a prior openStore/list touched the id).
		changed := false
		if m.ParentSession != "" && ex.ParentSession != m.ParentSession {
			ex.ParentSession = m.ParentSession
			changed = true
		}
		if m.Origin != "" && ex.Origin != m.Origin {
			ex.Origin = m.Origin
			changed = true
		}
		if m.DelegationDepth > 0 && ex.DelegationDepth != m.DelegationDepth {
			ex.DelegationDepth = m.DelegationDepth
			changed = true
		} else if changed && ex.ParentSession != "" && ex.DelegationDepth == 0 {
			if pm, ok := r.meta[normalizeID(ex.ParentSession)]; ok {
				ex.DelegationDepth = pm.DelegationDepth + 1
			} else {
				ex.DelegationDepth = 1
			}
		}
		if changed {
			r.meta[id] = ex
			r.mu.Unlock()
			if raw, err := json.Marshal(ex); err == nil {
				st := r.openStore(id)
				_, _ = st.append(struct {
					Type    string          `json:"type"`
					Role    string          `json:"role"`
					Content string          `json:"content"`
					Meta    json.RawMessage `json:"meta"`
				}{Type: "session_meta", Role: "host", Meta: raw})
			}
			return ex, false, nil
		}
		r.mu.Unlock()
		return ex, false, nil
	}
	// Inherit delegation depth from the parent Session when the caller left it unset.
	if m.ParentSession != "" && m.DelegationDepth == 0 {
		if pm, ok := r.meta[normalizeID(m.ParentSession)]; ok {
			m.DelegationDepth = pm.DelegationDepth + 1
		} else {
			m.DelegationDepth = 1
		}
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	r.meta[id] = m
	r.mu.Unlock()
	st := r.openStore(id)
	// Persist parent/origin/depth so restart keeps the Subagent tree.
	if raw, err := json.Marshal(m); err == nil {
		_, _ = st.append(struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content string          `json:"content"`
			Meta    json.RawMessage `json:"meta"`
		}{Type: "session_meta", Role: "host", Meta: raw})
	}
	return m, true, nil
}

func (r *registry) metaOf(id string) SessionMeta {
	id = normalizeID(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.meta[id]
}

func (st *store) load() {
	if st.path == "" {
		return
	}
	f, err := os.Open(st.path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var fact Fact
		if err := json.Unmarshal(line, &fact); err != nil {
			continue
		}
		st.facts = append(st.facts, fact)
	}
}

func (st *store) append(in struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"meta"`
}) (Fact, error) {
	if in.Role == "" {
		return Fact{}, &protocol.FrameError{Code: "bad_payload", Message: "role is required"}
	}
	if in.Type == "" {
		in.Type = "message"
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	f := Fact{
		Seq:     len(st.facts) + 1,
		Type:    in.Type,
		Role:    in.Role,
		Content: in.Content,
		Meta:    in.Meta,
		Ts:      time.Now().UnixMilli(),
	}
	st.facts = append(st.facts, f)
	if st.file != nil {
		raw, _ := json.Marshal(f)
		_, _ = st.file.Write(append(raw, '\n'))
	}
	return f, nil
}

func (st *store) query(afterSeq, limit int) []Fact {
	st.mu.Lock()
	defer st.mu.Unlock()
	var out []Fact
	for _, f := range st.facts {
		if f.Seq <= afterSeq {
			continue
		}
		out = append(out, f)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

type summaryMeta struct {
	Active           bool `json:"active"`
	CoversThroughSeq int  `json:"coversThroughSeq"`
}

func (st *store) derive() []Message {
	st.mu.Lock()
	defer st.mu.Unlock()

	// Latest active Context Summary replaces covered history (ADR-0013).
	var summaryContent string
	covers := 0
	for _, f := range st.facts {
		if f.Type != "context_summary" {
			continue
		}
		var m summaryMeta
		if len(f.Meta) > 0 {
			_ = json.Unmarshal(f.Meta, &m)
		}
		if !m.Active {
			continue
		}
		summaryContent = f.Content
		covers = m.CoversThroughSeq
	}

	var out []Message
	lastSystemIdx := -1
	for _, f := range st.facts {
		if f.Type == "context_summary" {
			// Summary is placed after the walk via summaryContent (kept as the
			// compacted history block, not competing with System Prompt stacking).
			continue
		}
		if f.Seq <= covers {
			continue
		}
		switch f.Type {
		case "message":
			if f.Role == "system" {
				// Single System Prompt projection: keep only the latest, in place.
				if lastSystemIdx >= 0 {
					out = append(out[:lastSystemIdx], out[lastSystemIdx+1:]...)
				}
				out = append(out, Message{Role: f.Role, Content: f.Content})
				lastSystemIdx = len(out) - 1
				continue
			}
			out = append(out, Message{Role: f.Role, Content: f.Content})
		case "tool_call":
			var meta struct {
				ToolCalls []struct {
					ID         string          `json:"id"`
					ToolCallID string          `json:"tool_call_id"`
					Name       string          `json:"name"`
					Arguments  json.RawMessage `json:"arguments"`
				} `json:"tool_calls"`
				ToolCallID string          `json:"tool_call_id"`
				Name       string          `json:"name"`
				Arguments  json.RawMessage `json:"arguments"`
			}
			if len(f.Meta) > 0 {
				_ = json.Unmarshal(f.Meta, &meta)
			}
			var calls []ToolCall
			for _, c := range meta.ToolCalls {
				id := c.ID
				if id == "" {
					id = c.ToolCallID
				}
				calls = append(calls, ToolCall{ID: id, Name: c.Name, Arguments: c.Arguments})
			}
			if len(calls) == 0 && meta.ToolCallID != "" {
				calls = []ToolCall{{ID: meta.ToolCallID, Name: meta.Name, Arguments: meta.Arguments}}
			}
			out = append(out, Message{Role: f.Role, Content: f.Content, ToolCalls: calls})
		case "tool_result":
			var meta struct {
				ToolCallID string `json:"tool_call_id"`
			}
			if len(f.Meta) > 0 {
				_ = json.Unmarshal(f.Meta, &meta)
			}
			out = append(out, Message{Role: f.Role, Content: f.Content, ToolCallID: meta.ToolCallID})
		}
	}
	if summaryContent != "" {
		// Compacted history sits before post-cover messages; if a System Prompt
		// already exists, keep summary as a system-role block after it.
		sum := Message{Role: "system", Content: summaryContent}
		if lastSystemIdx >= 0 {
			out = append(out[:lastSystemIdx+1], append([]Message{sum}, out[lastSystemIdx+1:]...)...)
		} else {
			out = append([]Message{sum}, out...)
		}
	}
	return out
}

func main() {
	s := pluginsdk.New()
	dir := dataDir()
	reg := newRegistry(dir)
	fmt.Fprintf(os.Stderr, "session: dataDir=%s\n", dir)

	// Current Session (ADR-0012): medium-agnostic current id on the session Capability.
	// Always a non-empty Session id (the implicit default is "default").
	var currentMu sync.Mutex
	currentID := defaultSessionID

	// listSessions walks the Session Log dir; title is the first user message
	// (truncated), seq the last fact's. Shared by the list Capability and /session list.
	listSessions := func() []sessionItem {
		var list []sessionItem
		entries, _ := os.ReadDir(reg.dataDir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".jsonl")
			st := reg.openStore(id)
			facts := st.query(0, 0)
			title := id
			seq := 0
			for _, f := range facts {
				seq = f.Seq
				if f.Type == "message" && f.Role == "user" && f.Content != "" {
					title = f.Content
					if len([]rune(title)) > 40 {
						title = string([]rune(title)[:40]) + "…"
					}
					break
				}
			}
			m := reg.metaOf(id)
			list = append(list, sessionItem{
				ID:              id,
				Title:           title,
				Seq:             seq,
				ParentSession:   m.ParentSession,
				Origin:          m.Origin,
				DelegationDepth: m.DelegationDepth,
			})
		}
		if list == nil {
			list = []sessionItem{}
		}
		return list
	}

	s.Handle("session", "create", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID       string `json:"sessionId"`
			ParentSession   string `json:"parentSession"`
			Origin          string `json:"origin"`
			DelegationDepth int    `json:"delegationDepth"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		if in.SessionID == "" {
			// Mint a fresh id (ADR-0012): empty create must not collapse onto "default".
			in.SessionID = fmt.Sprintf("s-%d", time.Now().UnixNano())
		}
		m, created, err := reg.create(in.SessionID, SessionMeta{
			ParentSession:   in.ParentSession,
			Origin:          in.Origin,
			DelegationDepth: in.DelegationDepth,
		})
		if err != nil {
			return nil, err
		}
		id := normalizeID(in.SessionID)
		// Subagent spawns must not steal Current Session from the parent (CONTEXT.md).
		// User-facing create (web "new chat") still selects the fresh id.
		if in.Origin != "subagent" {
			currentMu.Lock()
			currentID = id
			currentMu.Unlock()
		}
		return json.Marshal(map[string]any{
			"sessionId": id,
			"created":   created,
			"meta":      m,
		})
	})

	s.Handle("session", "append", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string          `json:"sessionId"`
			Type      string          `json:"type"`
			Role      string          `json:"role"`
			Content   string          `json:"content"`
			Meta      json.RawMessage `json:"meta"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		st := reg.openStore(in.SessionID)
		f, err := st.append(struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content string          `json:"content"`
			Meta    json.RawMessage `json:"meta"`
		}{Type: in.Type, Role: in.Role, Content: in.Content, Meta: in.Meta})
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]int{"seq": f.Seq})
	})

	s.Handle("session", "query", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
			AfterSeq  int    `json:"afterSeq"`
			Limit     int    `json:"limit"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		st := reg.openStore(in.SessionID)
		facts := st.query(in.AfterSeq, in.Limit)
		if facts == nil {
			facts = []Fact{}
		}
		return json.Marshal(map[string]any{"facts": facts})
	})

	s.Handle("session", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"sessions": listSessions()})
	})

	s.Handle("session", "current", func(req *pluginsdk.Request) (json.RawMessage, error) {
		currentMu.Lock()
		id := currentID
		currentMu.Unlock()
		return json.Marshal(map[string]any{"sessionId": id})
	})

	s.Handle("session", "select", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if in.SessionID == "" {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: "sessionId required"}
		}
		reg.openStore(in.SessionID)
		currentMu.Lock()
		currentID = in.SessionID
		currentMu.Unlock()
		return json.Marshal(map[string]any{"ok": true, "sessionId": in.SessionID})
	})

	s.Handle("session", "derive", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		st := reg.openStore(in.SessionID)
		msgs := stubOldToolResults(st.derive(), loadPluginConfig().FullToolResults)
		return json.Marshal(map[string]any{
			"messages": msgs,
			"count":    len(msgs),
		})
	})

	s.Handle("config", "get", func(req *pluginsdk.Request) (json.RawMessage, error) {
		cfg := loadPluginConfig()
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "fullToolResults", "value": cfg.FullToolResults, "type": "integer"},
			},
		})
	})
	s.Handle("config", "set", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in map[string]any
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		cfg := loadPluginConfig()
		for k, raw := range in {
			if k != "fullToolResults" {
				return nil, &protocol.FrameError{Code: "unknown_key", Message: "unknown config key " + k}
			}
			n, ok := raw.(float64)
			if !ok || n <= 0 {
				return nil, &protocol.FrameError{Code: "bad_arguments", Message: "fullToolResults must be a positive integer"}
			}
			cfg.FullToolResults = int(n)
		}
		raw, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(configPath(), raw, 0o600); err != nil {
			return nil, &protocol.FrameError{Code: "save_failed", Message: err.Error()}
		}
		return json.Marshal(map[string]any{"ok": true})
	})
	s.Handle("config", "schema", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "fullToolResults", "type": "integer", "default": defaultFullToolResults,
					"description": "Newest tool_result messages kept full in derive"},
			},
		})
	})
	s.Handle("config", "reload", func(req *pluginsdk.Request) (json.RawMessage, error) {
		cfg := loadPluginConfig()
		return json.Marshal(map[string]any{"ok": true, "fullToolResults": cfg.FullToolResults})
	})

	// Slash commands (ADR-0008): Session Log ops live on the session plugin, not Host natives.
	s.Handle("commands", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Command string `json:"command"`
			Args    string `json:"args"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		sid := strings.TrimSpace(in.Args)
		currentMu.Lock()
		if sid == "" {
			sid = currentID
		}
		currentMu.Unlock()
		switch in.Command {
		case "dump-trace":
			st := reg.openStore(sid)
			facts := st.query(0, 0)
			if facts == nil {
				facts = []Fact{}
			}
			raw, err := json.MarshalIndent(map[string]any{
				"sessionId":  sid,
				"exportedAt": time.Now().UTC().Format(time.RFC3339),
				"facts":      facts,
			}, "", "  ")
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"text": string(raw)})
		case "list":
			raw, _ := json.MarshalIndent(listSessions(), "", "  ")
			return json.Marshal(map[string]string{"text": string(raw)})
		case "derive":
			st := reg.openStore(sid)
			msgs := stubOldToolResults(st.derive(), loadPluginConfig().FullToolResults)
			raw, _ := json.MarshalIndent(msgs, "", "  ")
			return json.Marshal(map[string]string{"text": string(raw)})
		case "current":
			currentMu.Lock()
			id := currentID
			currentMu.Unlock()
			return json.Marshal(map[string]string{"text": "sessionId=" + id})
		default:
			return nil, &protocol.FrameError{
				Code:    "unknown_command",
				Message: fmt.Sprintf("unknown command %q (try dump-trace|list|derive|current)", in.Command),
			}
		}
	})

	_ = s.Serve()
}
