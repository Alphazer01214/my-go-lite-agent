// Command session is an append-only Session Log Plugin (in-memory storage).
//
// Capability: session
//   - create:  create a Session by id (idempotent); optional parentSession/origin/delegationDepth
//   - append:  append one fact to a Session; returns {seq}
//   - query:   list facts (optional sessionId/afterSeq/limit)
//   - derive:  rebuild Model Context messages from one Session log (pure projection)
//
// Empty sessionId targets the default Session (backward compatible).
// Storage lives only in this process; swap the Plugin to replace the backend.
package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const defaultSessionID = "default"

// Fact is one append-only Session Log entry.
type Fact struct {
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

// Message is one model-visible chat message produced by derive.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a model-requested tool invocation projected from the log.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// SessionMeta is storage metadata outside the fact log.
type SessionMeta struct {
	CreatedAt       int64  `json:"createdAt"`
	ParentSession   string `json:"parentSession,omitempty"`
	Origin          string `json:"origin,omitempty"`
	DelegationDepth int    `json:"delegationDepth,omitempty"`
}

type store struct {
	mu    sync.Mutex
	facts []Fact
}

type registry struct {
	mu       sync.Mutex
	sessions map[string]*store
	meta     map[string]SessionMeta
}

func newRegistry() *registry {
	r := &registry{
		sessions: make(map[string]*store),
		meta:     make(map[string]SessionMeta),
	}
	r.sessions[defaultSessionID] = &store{}
	r.meta[defaultSessionID] = SessionMeta{CreatedAt: time.Now().UnixMilli()}
	return r
}

func normalizeID(id string) string {
	if id == "" {
		return defaultSessionID
	}
	return id
}

func (r *registry) getOrCreate(id string) (*store, SessionMeta, bool) {
	id = normalizeID(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.sessions[id]
	if ok {
		return st, r.meta[id], false
	}
	st = &store{}
	r.sessions[id] = st
	m := SessionMeta{CreatedAt: time.Now().UnixMilli()}
	r.meta[id] = m
	return st, m, true
}

func (r *registry) create(id string, m SessionMeta) (SessionMeta, bool, error) {
	id = normalizeID(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.sessions[id]; ok {
		_ = st
		return r.meta[id], false, nil
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	r.sessions[id] = &store{}
	r.meta[id] = m
	return m, true, nil
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

func (st *store) derive() []Message {
	st.mu.Lock()
	defer st.mu.Unlock()
	msgs := make([]Message, 0, len(st.facts))
	for _, f := range st.facts {
		switch f.Type {
		case "message":
			msgs = append(msgs, Message{Role: f.Role, Content: f.Content})
		case "tool_call":
			var meta struct {
				ToolCalls []struct {
					// Host writes tool_call_id; ToolCall JSON uses id. Accept both.
					ID         string          `json:"id"`
					ToolCallID string          `json:"tool_call_id"`
					Name       string          `json:"name"`
					Arguments  json.RawMessage `json:"arguments"`
				} `json:"tool_calls"`
				// Legacy single-call shape.
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
				calls = []ToolCall{{
					ID:        meta.ToolCallID,
					Name:      meta.Name,
					Arguments: meta.Arguments,
				}}
			}
			msgs = append(msgs, Message{
				Role:      f.Role,
				Content:   f.Content,
				ToolCalls: calls,
			})
		case "tool_result":
			var meta struct {
				ToolCallID string `json:"tool_call_id"`
			}
			if len(f.Meta) > 0 {
				_ = json.Unmarshal(f.Meta, &meta)
			}
			msgs = append(msgs, Message{
				Role:       f.Role,
				Content:    f.Content,
				ToolCallID: meta.ToolCallID,
			})
		}
	}
	return msgs
}

func main() {
	s := pluginsdk.New()
	reg := newRegistry()

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
		m, created, err := reg.create(in.SessionID, SessionMeta{
			ParentSession:   in.ParentSession,
			Origin:          in.Origin,
			DelegationDepth: in.DelegationDepth,
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{
			"sessionId": normalizeID(in.SessionID),
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
		st, _, _ := reg.getOrCreate(in.SessionID)
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
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		st, _, _ := reg.getOrCreate(in.SessionID)
		facts := st.query(in.AfterSeq, in.Limit)
		if facts == nil {
			facts = []Fact{}
		}
		return json.Marshal(map[string]any{"facts": facts})
	})

	s.Handle("session", "derive", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		st, _, _ := reg.getOrCreate(in.SessionID)
		msgs := st.derive()
		return json.Marshal(map[string]any{
			"messages": msgs,
			"count":    len(msgs),
		})
	})

	_ = s.Serve()
}
