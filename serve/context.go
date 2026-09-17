package serve

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// callByCapOwner looks up the unique provides owner and point-names it
// (Host-internal helper for the deferred agent.* special case only).
func (s *Server) callByCapOwner(cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	owner, ok := s.provides[cap]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no plugin provides %q", cap)
	}
	return s.callByPlugin(owner, cap, method, payload)
}

// agentDerive rebuilds Model Context through the Session Plugin's public
// session.derive contract (empty sessionID targets the default Session).
func (s *Server) agentDerive(sessionID string) ([]Message, error) {
	payload := json.RawMessage(`{}`)
	if sessionID != "" {
		payload = MarshalPayload(map[string]string{"sessionId": sessionID})
	}
	out, err := s.callByCapOwner(pluginsdk.SessionCap, "derive", payload)
	if err != nil {
		return nil, fmt.Errorf("session.derive: %w", err)
	}
	var res struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("session.derive: bad payload: %w", err)
	}
	if res.Messages == nil {
		res.Messages = []Message{}
	}
	return res.Messages, nil
}

// agentAppend appends one fact through the Session Plugin's public
// session.append contract and returns the new seq.
func (s *Server) agentAppend(sessionID string, fact map[string]any) (int, error) {
	out, err := s.callByCapOwner(pluginsdk.SessionCap, "append", MarshalPayload(fact))
	if err != nil {
		return 0, fmt.Errorf("session.append: %w", err)
	}
	var res struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return 0, fmt.Errorf("session.append: bad payload: %w", err)
	}
	return res.Seq, nil
}

// AgentRequestResult is the validated Model Context after the log invariant check.
type AgentRequestResult struct {
	Messages []Message `json:"messages"`
	// Rebuilt is true when Host rebuilt Model Context from the log (empty claimed).
	// When claimed was supplied and matched, Rebuilt is false (validated, not rebuilt).
	Rebuilt bool `json:"rebuilt"`
}

// AgentRequest enforces the Session Log invariant before a model call (ADR-0002).
//
// claimed empty/nil → rebuild from session.derive and accept.
// claimed non-empty → must match session.derive exactly, else session_invariant_violation.
// Empty sessionID targets the default Session.
func (s *Server) AgentRequest(sessionID string, claimed []Message) (*AgentRequestResult, error) {
	derived, err := s.agentDerive(sessionID)
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
	last := 0
	for _, m := range msgs {
		fact := map[string]any{
			"type":    "message",
			"role":    defaultSystemRole(m.Role),
			"content": m.Content,
		}
		if in.SessionID != "" {
			fact["sessionId"] = in.SessionID
		}
		seq, err := s.agentAppend(in.SessionID, fact)
		if err != nil {
			return nil, err
		}
		last = seq
	}
	return map[string]int{"count": len(msgs), "lastSeq": last}, nil
}

func defaultSystemRole(role string) string {
	if role == "" {
		return "system"
	}
	return role
}
