// Command context-manager is the Context Manager Plugin (CONTEXT.md).
//
// Capability: system-prompt
//   - registerSegment / registerContext / assemble
//
// Capability: context
//   - prepare:  {sessionId, messages} → {messages, tools, systemText, usage, compactHint}
//   - compact:  {messages, coversThroughSeq} → {summary, coversThroughSeq}
//   - usage:    last prepare usage for a session
//   - listContext: last prepare messages preview
//   - registerSkill: skill catalog segment (trigger injection is a later feature)
//
// Optional static base segments load from segments.json beside the executable.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// Segment is one Prompt Segment (CONTEXT.md).
type Segment struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
	Text  string `json:"text"`
}

// message mirrors Host serve.Message so prepare can round-trip history
// without dropping tool_calls (session_invariant_violation otherwise).
type toolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type usageInfo struct {
	EstimatedTokens int    `json:"estimatedTokens"`
	Chars           int    `json:"chars"`
	MessageCount    int    `json:"messageCount"`
	Source          string `json:"source"`
}

type sessionState struct {
	usage    usageInfo
	messages []message
}

type store struct {
	mu       sync.Mutex
	segments map[string]Segment
	contexts map[string]Segment
	skills   map[string]Segment
	session  map[string]*sessionState
}

func newStore() *store {
	return &store{
		segments: make(map[string]Segment),
		contexts: make(map[string]Segment),
		skills:   make(map[string]Segment),
		session:  make(map[string]*sessionState),
	}
}

func (st *store) loadBaseFile() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "segments.json"))
	if err != nil {
		return
	}
	var file struct {
		Segments []Segment `json:"segments"`
		Contexts []Segment `json:"contexts"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, seg := range file.Segments {
		if seg.Name != "" {
			st.segments[seg.Name] = seg
		}
	}
	for _, seg := range file.Contexts {
		if seg.Name != "" {
			st.contexts[seg.Name] = seg
		}
	}
}

func (st *store) registerSegment(seg Segment) error {
	if seg.Name == "" {
		return &protocol.FrameError{Code: "bad_payload", Message: "name is required"}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.segments[seg.Name] = seg
	return nil
}

func (st *store) registerContext(seg Segment) error {
	if seg.Name == "" {
		return &protocol.FrameError{Code: "bad_payload", Message: "name is required"}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.contexts[seg.Name] = seg
	return nil
}

func (st *store) registerSkill(seg Segment) error {
	if seg.Name == "" {
		return &protocol.FrameError{Code: "bad_payload", Message: "name is required"}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if seg.Order == 0 {
		seg.Order = 200
	}
	if seg.Text == "" {
		seg.Text = seg.Name
	}
	st.skills[seg.Name] = seg
	return nil
}

func sortByOrderName(items []Segment) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Order != items[j].Order {
			return items[i].Order < items[j].Order
		}
		return items[i].Name < items[j].Name
	})
}

func joinParts(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(p)
		_ = i
	}
	return b.String()
}

func (st *store) assemble() (text string, segments []Segment) {
	st.mu.Lock()
	defer st.mu.Unlock()
	segments = make([]Segment, 0, len(st.segments)+len(st.skills))
	for _, seg := range st.segments {
		segments = append(segments, seg)
	}
	for _, seg := range st.skills {
		segments = append(segments, seg)
	}
	sortByOrderName(segments)

	ctx := make([]Segment, 0, len(st.contexts))
	for _, seg := range st.contexts {
		ctx = append(ctx, seg)
	}
	sortByOrderName(ctx)

	var parts []string
	for _, seg := range segments {
		if seg.Text != "" {
			parts = append(parts, seg.Text)
		}
	}
	for _, seg := range ctx {
		if seg.Text != "" {
			parts = append(parts, seg.Text)
		}
	}
	return joinParts(parts), segments
}

func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

// modelVisibleChars counts only what the LLM hop actually receives:
// message.content plus tool_call name/arguments. Host meta / framing JSON is excluded.
// Do not add systemText on top — derive messages already include the System Prompt.
func modelVisibleChars(msgs []message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Name) + len(tc.Arguments)
		}
	}
	return n
}

func buildSummary(msgs []message) string {
	var lines []string
	lines = append(lines, "Conversation summary (compacted):")
	for _, m := range msgs {
		c := strings.TrimSpace(m.Content)
		if c == "" && len(m.ToolCalls) > 0 {
			c = "tool_call " + m.ToolCalls[0].Name
		}
		c = strings.ReplaceAll(c, "\n", " ")
		r := []rune(c)
		if len(r) > 120 {
			c = string(r[:120]) + "…"
		}
		if c == "" {
			c = "(empty)"
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", m.Role, c))
	}
	return strings.Join(lines, "\n")
}

func main() {
	s := pluginsdk.New()
	st := newStore()
	st.loadBaseFile()

	listTools := func() []map[string]any {
		raw, err := s.Call("tools", "list", json.RawMessage(`{}`))
		if err != nil {
			return nil
		}
		var out struct {
			Tools []map[string]any `json:"tools"`
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &out)
		}
		if out.Tools == nil {
			return []map[string]any{}
		}
		return out.Tools
	}

	s.Handle("system-prompt", "registerSegment", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var seg Segment
		if err := json.Unmarshal(req.Payload, &seg); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if err := st.registerSegment(seg); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"name": seg.Name})
	})

	s.Handle("system-prompt", "registerContext", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var seg Segment
		if err := json.Unmarshal(req.Payload, &seg); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if err := st.registerContext(seg); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"name": seg.Name})
	})

	s.Handle("system-prompt", "assemble", func(req *pluginsdk.Request) (json.RawMessage, error) {
		text, segments := st.assemble()
		return json.Marshal(map[string]any{
			"text":     text,
			"segments": segments,
		})
	})

	s.Handle("context", "registerSkill", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var seg Segment
		if err := json.Unmarshal(req.Payload, &seg); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if err := st.registerSkill(seg); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"name": seg.Name})
	})

	s.Handle("context", "prepare", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string    `json:"sessionId"`
			Messages  []message `json:"messages"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		systemText, segments := st.assemble()
		tools := listTools()
		if tools == nil {
			tools = []map[string]any{}
		}
		// Model-visible content only (no double-count of systemText, no Host meta).
		chars := modelVisibleChars(in.Messages)
		u := usageInfo{
			Chars:           chars,
			EstimatedTokens: estimateTokens(chars),
			MessageCount:    len(in.Messages),
			Source:          "chars",
		}
		sid := in.SessionID
		if sid == "" {
			sid = "default"
		}
		st.mu.Lock()
		// Always refresh chars/messageCount from this prepare. Keep provider
		// tokens as the preferred token figure when available (CONTEXT.md Usage).
		var providerTokens int
		if prev := st.session[sid]; prev != nil && prev.usage.Source == "provider" {
			providerTokens = prev.usage.EstimatedTokens
		}
		st.session[sid] = &sessionState{usage: u, messages: in.Messages}
		st.mu.Unlock()
		if providerTokens > 0 {
			u.Source = "provider"
			u.EstimatedTokens = providerTokens
			st.mu.Lock()
			st.session[sid].usage = u
			st.mu.Unlock()
		}

		// compactHint when the estimated hop is large (Host still owns the final budget check).
		hint := u.EstimatedTokens > 4000 || chars > 16000

		return json.Marshal(map[string]any{
			"messages":    in.Messages,
			"tools":       tools,
			"systemText":  systemText,
			"segments":    segments,
			"usage":       u,
			"compactHint": hint,
		})
	})

	s.Handle("context", "noteUsage", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string         `json:"sessionId"`
			Usage     map[string]any `json:"usage"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		if len(in.Usage) == 0 {
			return json.Marshal(map[string]any{"ok": false})
		}
		sid := in.SessionID
		if sid == "" {
			sid = "default"
		}
		providerTokens := 0
		if v, ok := in.Usage["total_tokens"].(float64); ok {
			providerTokens = int(v)
		} else if v, ok := in.Usage["totalTokens"].(float64); ok {
			providerTokens = int(v)
		}
		st.mu.Lock()
		prev := st.session[sid]
		if prev == nil {
			prev = &sessionState{}
			st.session[sid] = prev
		}
		prev.usage.Source = "provider"
		if providerTokens > 0 {
			prev.usage.EstimatedTokens = providerTokens
		}
		st.mu.Unlock()
		return json.Marshal(map[string]any{"ok": true, "source": "provider"})
	})

	s.Handle("context", "compact", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID        string    `json:"sessionId"`
			Messages         []message `json:"messages"`
			CoversThroughSeq int       `json:"coversThroughSeq"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		summary := buildSummary(in.Messages)
		return json.Marshal(map[string]any{
			"summary":          summary,
			"coversThroughSeq": in.CoversThroughSeq,
		})
	})

	s.Handle("context", "usage", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		sid := in.SessionID
		if sid == "" {
			sid = "default"
		}
		st.mu.Lock()
		sess := st.session[sid]
		st.mu.Unlock()
		if sess == nil {
			return json.Marshal(map[string]any{"usage": nil})
		}
		return json.Marshal(map[string]any{"usage": sess.usage})
	})

	s.Handle("context", "listContext", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			SessionID string `json:"sessionId"`
			N         int    `json:"n"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		sid := in.SessionID
		if sid == "" {
			sid = "default"
		}
		n := in.N
		if n <= 0 {
			n = 20
		}
		st.mu.Lock()
		sess := st.session[sid]
		st.mu.Unlock()
		if sess == nil {
			return json.Marshal(map[string]any{"messages": []message{}})
		}
		msgs := sess.messages
		if len(msgs) > n {
			msgs = msgs[len(msgs)-n:]
		}
		return json.Marshal(map[string]any{"messages": msgs, "count": len(msgs)})
	})

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
		if sid == "" {
			sid = "default"
		}
		switch in.Command {
		case "usage":
			st.mu.Lock()
			sess := st.session[sid]
			st.mu.Unlock()
			if sess == nil {
				return json.Marshal(map[string]string{"text": "(no usage yet — run a turn first)"})
			}
			raw, _ := json.MarshalIndent(sess.usage, "", "  ")
			return json.Marshal(map[string]string{"text": string(raw)})
		case "list":
			n := 10
			st.mu.Lock()
			sess := st.session[sid]
			st.mu.Unlock()
			if sess == nil || len(sess.messages) == 0 {
				return json.Marshal(map[string]string{"text": "(no prepare messages yet)"})
			}
			msgs := sess.messages
			if len(msgs) > n {
				msgs = msgs[len(msgs)-n:]
			}
			raw, _ := json.MarshalIndent(msgs, "", "  ")
			return json.Marshal(map[string]string{"text": string(raw)})
		case "skills":
			st.mu.Lock()
			var names []string
			for _, sk := range st.skills {
				names = append(names, sk.Name)
			}
			st.mu.Unlock()
			if len(names) == 0 {
				return json.Marshal(map[string]string{"text": "(no skills registered)"})
			}
			sort.Strings(names)
			return json.Marshal(map[string]string{"text": strings.Join(names, "\n")})
		default:
			return nil, &protocol.FrameError{
				Code:    "unknown_command",
				Message: fmt.Sprintf("unknown command %q (try usage|list|skills)", in.Command),
			}
		}
	})

	_ = s.Serve()
}
