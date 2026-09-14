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

type Fact struct {
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"meta,omitempty"`
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
	return st
}

func (r *registry) getOrCreate(id string) (*store, SessionMeta, bool) {
	id = normalizeID(id)
	st := r.openStore(id)
	r.mu.Lock()
	m := r.meta[id]
	_, existed := r.meta[id]
	r.mu.Unlock()
	return st, m, !existed
}

func (r *registry) create(id string, m SessionMeta) (SessionMeta, bool, error) {
	id = normalizeID(id)
	r.mu.Lock()
	if ex, ok := r.meta[id]; ok {
		r.mu.Unlock()
		return ex, false, nil
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	r.meta[id] = m
	r.mu.Unlock()
	_ = r.openStore(id)
	return m, true, nil
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
	var currentMu sync.Mutex
	currentID := ""

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
		currentMu.Lock()
		currentID = id
		currentMu.Unlock()
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
		type item struct {
			ID    string `json:"id"`
			Title string `json:"title,omitempty"`
			Seq   int    `json:"seq,omitempty"`
		}
		var list []item
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
			list = append(list, item{ID: id, Title: title, Seq: seq})
		}
		if list == nil {
			list = []item{}
		}
		return json.Marshal(map[string]any{"sessions": list})
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
		msgs := st.derive()
		return json.Marshal(map[string]any{
			"messages": msgs,
			"count":    len(msgs),
		})
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
				"sessionId": sid,
				"exportedAt": time.Now().UTC().Format(time.RFC3339),
				"facts":      facts,
			}, "", "  ")
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"text": string(raw)})
		case "list":
			// Reuse list handler shape via store walk.
			type item struct {
				ID    string `json:"id"`
				Title string `json:"title,omitempty"`
				Seq   int    `json:"seq,omitempty"`
			}
			var list []item
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
				list = append(list, item{ID: id, Title: title, Seq: seq})
			}
			if list == nil {
				list = []item{}
			}
			raw, _ := json.MarshalIndent(list, "", "  ")
			return json.Marshal(map[string]string{"text": string(raw)})
		case "derive":
			st := reg.openStore(sid)
			msgs := st.derive()
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
