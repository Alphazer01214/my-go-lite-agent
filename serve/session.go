package serve

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// sessionTurn is one Session's in-flight default-Loop turn control.
// mu serializes turns on the same Session; different Sessions run in parallel.
type sessionTurn struct {
	mu      sync.Mutex
	cancel  atomic.Bool
	running atomic.Bool
}

// Message is one model-visible chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// TurnResult is one Agent Loop turn returned by the mounted loop provider (ADR-0016).
type TurnResult struct {
	User      string    `json:"user"`
	Assistant string    `json:"assistant"`
	Chunks    []string  `json:"chunks"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
	Messages  []Message `json:"messages"`
}

// normalizeSessionID maps empty/blank to the default Session id so Host turn
// state, status, and render tags never split "" vs "default" into two Sessions.
func normalizeSessionID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "default"
	}
	return id
}

func (s *Server) turnFor(sid string) *sessionTurn {
	sid = normalizeSessionID(sid)
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	if s.turnStates == nil {
		s.turnStates = make(map[string]*sessionTurn)
	}
	st := s.turnStates[sid]
	if st == nil {
		st = &sessionTurn{}
		s.turnStates[sid] = st
	}
	return st
}

// acquireTurn TryLocks the Session turn. Returns nil if that Session is busy.
func (s *Server) acquireTurn(sid string) *sessionTurn {
	st := s.turnFor(sid)
	if !st.mu.TryLock() {
		return nil
	}
	st.cancel.Store(false)
	return st
}

func (s *Server) beginRunning(sid string) {
	s.turnFor(sid).running.Store(true)
}

func (s *Server) endRunning(sid string) {
	s.turnFor(sid).running.Store(false)
}

// isRunning reports whether any default-Loop turn is in flight (internal;
// Mediums observe per-Session via RunningSessions / IsRunningOn).
func (s *Server) isRunning() bool {
	return len(s.RunningSessions()) > 0
}

// IsRunningOn reports whether sid itself has an in-flight turn.
func (s *Server) IsRunningOn(sid string) bool {
	sid = normalizeSessionID(sid)
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	st := s.turnStates[sid]
	return st != nil && st.running.Load()
}

// RunningSessions lists Session ids with an in-flight turn.
func (s *Server) RunningSessions() []string {
	s.turnStatesMu.Lock()
	defer s.turnStatesMu.Unlock()
	var out []string
	for sid, st := range s.turnStates {
		if st.running.Load() {
			out = append(out, sid)
		}
	}
	return out
}

// StatusForSession reports "running" when sid owns an in-flight turn, else "idle".
func (s *Server) StatusForSession(sid string) string {
	if s.IsRunningOn(sid) {
		return "running"
	}
	return "idle"
}

func (s *Server) emitStatus(sessionID, status string) {
	sessionID = normalizeSessionID(sessionID)
	s.publish(Event{Topic: "status", Data: map[string]string{"status": status, "sessionId": sessionID}})
}

// RunTurn is the Host entry for one chat turn: lock + status + loop.turn (ADR-0016).
//
// Default flow: turn/start → system-prompt.assemble → session.append(system) → session.append(user)
// → per Step: step/start → AgentRequest(rebuild) → llm.complete(tools)
// → optional tools.call → session tool facts → step/end → repeat until final assistant reply
// → turn/end.
func (s *Server) RunTurn(userInput string) (*TurnResult, error) {
	return s.RunTurnOn("", userInput)
}

// RunTurnOn is RunTurn bound to a Session id (empty = default).
// Turns on different Sessions run in parallel; the same Session stays serial.
func (s *Server) RunTurnOn(sessionID, userInput string) (*TurnResult, error) {
	return s.runTurn(normalizeSessionID(sessionID), userInput, true, "")
}

// CancelTurnOn requests the in-flight Loop on sessionID to stop at the next
// safe boundary. Also notifies the mounted Agent Plugin so an external Loop can
// observe cancel (ADR-0016). The notification is a fire-and-forget req (no
// pending registered, response ignored): spec permits either TypeEvt or a
// pending-registered req — fire-and-forget keeps the cancel path non-blocking
// and is covered by lifecycle tests.
func (s *Server) CancelTurnOn(sessionID string) {
	sessionID = normalizeSessionID(sessionID)
	s.turnStatesMu.Lock()
	st := s.turnStates[sessionID]
	s.turnStatesMu.Unlock()
	if st != nil {
		st.cancel.Store(true)
	}
	s.mu.Lock()
	loopOwner, ok := s.provides[LoopCap]
	s.mu.Unlock()
	if ok {
		payload := MarshalPayload(map[string]any{"sessionId": sessionID})
		_ = s.writeTo(loopOwner, &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     LoopCap,
			Method:  "cancel",
			Payload: payload,
		})
	}
}

// TurnCancelledOn reports whether CancelTurnOn was requested for sessionID.
func (s *Server) TurnCancelledOn(sessionID string) bool {
	sessionID = normalizeSessionID(sessionID)
	s.turnStatesMu.Lock()
	st := s.turnStates[sessionID]
	s.turnStatesMu.Unlock()
	return st != nil && st.cancel.Load()
}

// runTurn executes one Turn on sessionID via the mounted Agent Plugin (ADR-0016).
// Host owns the per-session lock, Cancel/running status; Loop policy lives in the agent.
func (s *Server) runTurn(sessionID, userInput string, allowSubagent bool, extraSystem string) (result *TurnResult, err error) {
	if strings.TrimSpace(userInput) == "" {
		return nil, fmt.Errorf("agent loop: user input is required")
	}

	st := s.acquireTurn(sessionID)
	if st == nil {
		return nil, fmt.Errorf("session already has a running turn")
	}
	defer st.mu.Unlock()

	s.mu.Lock()
	loopOwner, hasExternalLoop := s.provides[LoopCap]
	s.mu.Unlock()
	if !hasExternalLoop {
		return nil, fmt.Errorf("agent loop: no plugin provides %q (mount an Agent plugin)", LoopCap)
	}

	s.beginRunning(sessionID)
	s.emitStatus(sessionID, "running")
	status := "idle"
	defer func() {
		s.endRunning(sessionID)
		s.emitStatus(sessionID, status)
	}()

	res, err := s.runExternalTurn(loopOwner, sessionID, userInput, allowSubagent, extraSystem)
	if err != nil {
		if st.cancel.Load() {
			status = "idle"
			return nil, fmt.Errorf("turn cancelled")
		}
		status = "error"
		if err.Error() != "" {
			status = "error: " + err.Error()
		}
		return nil, err
	}
	return res, nil
}

func (s *Server) runExternalTurn(owner, sessionID, userInput string, allowSubagent bool, extraSystem string) (*TurnResult, error) {
	payload := map[string]any{"input": userInput, "allowSubagent": allowSubagent}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	if extraSystem != "" {
		payload["extraSystem"] = extraSystem
	}
	res, err := s.call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     LoopCap,
		Method:  "turn",
		Payload: MarshalPayload(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	if res.Error != nil {
		return nil, fmt.Errorf("agent loop: %w", res.Error)
	}
	var out TurnResult
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return nil, fmt.Errorf("agent loop: bad loop.turn payload: %w", err)
		}
	}
	if out.User == "" {
		out.User = userInput
	}
	return &out, nil
}

func messagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
		if a[i].ToolCallID != b[i].ToolCallID {
			return false
		}
		if len(a[i].ToolCalls) != len(b[i].ToolCalls) {
			return false
		}
		for j := range a[i].ToolCalls {
			x, y := a[i].ToolCalls[j], b[i].ToolCalls[j]
			if x.ID != y.ID || x.Name != y.Name || !bytesEqualJSON(x.Arguments, y.Arguments) {
				return false
			}
		}
	}
	return true
}

func bytesEqualJSON(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return string(a) == string(b)
}
