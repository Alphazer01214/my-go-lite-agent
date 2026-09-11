// Command session is an append-only Session Log Plugin (in-memory storage).
//
// Capability: session
//   - append: append one fact; returns {seq}
//   - query:  list facts (optional afterSeq/limit)
//   - derive: rebuild Model Context messages from the log (pure projection)
//
// Storage lives only in this process; swap the Plugin to replace the backend.
package main

import (
	"encoding/json"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

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

type store struct {
	mu    sync.Mutex
	facts []Fact
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
				ToolCalls []ToolCall `json:"tool_calls"`
				// Legacy single-call shape.
				ToolCallID string          `json:"tool_call_id"`
				Name       string          `json:"name"`
				Arguments  json.RawMessage `json:"arguments"`
			}
			if len(f.Meta) > 0 {
				_ = json.Unmarshal(f.Meta, &meta)
			}
			calls := meta.ToolCalls
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
	st := &store{}

	s.Handle("session", "append", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content string          `json:"content"`
			Meta    json.RawMessage `json:"meta"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		f, err := st.append(in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]int{"seq": f.Seq})
	})

	s.Handle("session", "query", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			AfterSeq int `json:"afterSeq"`
			Limit    int `json:"limit"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		facts := st.query(in.AfterSeq, in.Limit)
		if facts == nil {
			facts = []Fact{}
		}
		return json.Marshal(map[string]any{"facts": facts})
	})

	s.Handle("session", "derive", func(req *pluginsdk.Request) (json.RawMessage, error) {
		msgs := st.derive()
		return json.Marshal(map[string]any{
			"messages": msgs,
			"count":    len(msgs),
		})
	})

	_ = s.Serve()
}
