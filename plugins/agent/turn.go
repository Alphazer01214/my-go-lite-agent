package main

import (
	"encoding/json"
	"sync"
)

type turnState struct {
	sessionID string
	mu        sync.Mutex
	cancelled bool
	done      chan struct{}
}

func newTurnState(id string) *turnState {
	return &turnState{sessionID: id, done: make(chan struct{})}
}

func (t *turnState) isCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

func (t *turnState) markCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled {
		return false
	}
	t.cancelled = true
	close(t.done)
	return true
}

type emitFunc func(payload json.RawMessage) error

// turnWithEmit binds stream emit to the inbound loop.turn req id.
func (p *plugin) turnWithEmit(reqID string, in turnIn) (turnOut, error) {
	emit := func(payload json.RawMessage) error {
		return p.sdk.EmitWithID(reqID, "loop", "chunk", payload)
	}
	return p.turn(in, emit)
}

func (p *plugin) turn(in turnIn, emit emitFunc) (turnOut, error) {
	if in.Input == "" {
		return turnOut{}, errBadArguments("input is required")
	}
	cfg := p.snapshot()
	maxSteps := in.MaxSteps
	if maxSteps <= 0 {
		maxSteps = cfg.MaxSteps
	}
	if maxSteps <= 0 {
		maxSteps = 128
	}
	scheme := maskScheme(cfg.DefaultScheme)

	sessionID := in.SessionID
	var err error
	if sessionID == "" {
		sessionID, err = p.createSession(in.Workspace)
		if err != nil {
			return turnOut{}, err
		}
	}

	slot := p.slotFor(sessionID)
	slot.turnMu.Lock()
	defer slot.turnMu.Unlock()

	st := newTurnState(sessionID)
	slot.mu.Lock()
	slot.active = st
	slot.mu.Unlock()
	defer func() {
		slot.mu.Lock()
		slot.active = nil
		slot.mu.Unlock()
	}()

	p.beginWork()
	defer p.endWork()

	return p.runTurn(st, turnIn{
		Input:        in.Input,
		SessionID:    sessionID,
		Workspace:    in.Workspace,
		MaxSteps:     maxSteps,
		AllowedTools: in.AllowedTools,
	}, scheme, maxSteps, emit)
}

func (p *plugin) cancel(in cancelIn) (cancelOut, error) {
	if in.SessionID == "" {
		return cancelOut{}, errBadArguments("session_id is required")
	}
	slot := p.slotFor(in.SessionID)
	slot.mu.Lock()
	active := slot.active
	slot.mu.Unlock()
	if active == nil {
		return cancelOut{SessionID: in.SessionID, Cancelled: false}, nil
	}
	active.markCancelled()
	return cancelOut{SessionID: in.SessionID, Cancelled: true}, nil
}

func (p *plugin) createSession(workspace string) (string, error) {
	raw, err := p.sdk.Call("session", "create", mustJSON(sessionCreateIn{Workspace: workspace}))
	if err != nil {
		return "", err
	}
	var out sessionCreateOut
	if err := json.Unmarshal(raw, &out); err != nil || out.SessionID == "" {
		return "", errHandler("bad session.create response")
	}
	return out.SessionID, nil
}

func (p *plugin) appendFacts(sessionID string, facts []map[string]any) ([]int, error) {
	raw, err := p.sdk.Call("session", "append", mustJSON(sessionAppendIn{SessionID: sessionID, Facts: facts}))
	if err != nil {
		return nil, err
	}
	var out sessionAppendOut
	_ = json.Unmarshal(raw, &out)
	return out.Seqs, nil
}

func (p *plugin) assembleMessages(sessionID string) ([]chatMessage, error) {
	raw, err := p.sdk.Call("memory", "assemble", mustJSON(map[string]string{"session_id": sessionID}))
	if err == nil {
		var out messagesOut
		if json.Unmarshal(raw, &out) == nil && len(out.Messages) > 0 {
			return out.Messages, nil
		}
	}
	raw, err = p.sdk.Call("session", "derive", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return nil, err
	}
	var out messagesOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errHandler("bad session.derive response")
	}
	return out.Messages, nil
}

func (p *plugin) collectTools(scheme string, allowed []string) ([]toolSpec, []byte, error) {
	if len(allowed) == 0 && scheme == schemeChat {
		return nil, nil, nil
	}
	raw, err := p.sdk.Call("tools", "list", mustJSON(map[string]any{}))
	if err != nil {
		return nil, nil, nil // tools optional
	}
	var listed struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, nil, nil
	}
	filtered := filterTools(listed.Tools, scheme, allowed)
	if len(filtered) == 0 {
		return nil, nil, nil
	}
	oa := make([]map[string]any, 0, len(filtered))
	for _, t := range filtered {
		params := any(t.Parameters)
		if len(t.Parameters) == 0 {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		oa = append(oa, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return filtered, mustJSON(oa), nil
}

func (p *plugin) callTool(name, arguments, workspace, callID string) (string, error) {
	args := json.RawMessage(arguments)
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	raw, err := p.sdk.Call("tools", "call", mustJSON(map[string]any{
		"name":      name,
		"arguments": args,
		"workspace": workspace,
		"call_id":   callID,
	}))
	if err != nil {
		return "", err
	}
	var out struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Content, nil
}

func (p *plugin) runTurn(st *turnState, in turnIn, scheme string, maxSteps int, emit emitFunc) (turnOut, error) {
	out := turnOut{SessionID: in.SessionID}
	seqs := make([]int, 0, 16)

	s, err := p.appendFacts(in.SessionID, []map[string]any{{
		"type": "turn_start", "role": "host", "meta": map[string]any{"workspace": in.Workspace},
	}})
	if err != nil {
		return out, err
	}
	seqs = append(seqs, s...)

	s, err = p.appendFacts(in.SessionID, []map[string]any{{
		"type": "message", "role": "user", "content": in.Input,
	}})
	if err != nil {
		return out, err
	}
	seqs = append(seqs, s...)

	_, toolsJSON, err := p.collectTools(scheme, in.AllowedTools)
	if err != nil {
		return out, err
	}

	for step := 1; step <= maxSteps; step++ {
		if st.isCancelled() {
			out.Cancelled = true
			break
		}
		out.Steps = step

		msgs, err := p.assembleMessages(in.SessionID)
		if err != nil {
			return out, err
		}

		s, err = p.appendFacts(in.SessionID, []map[string]any{
			{"type": "step_start", "role": "host", "meta": map[string]any{"step": step}},
			{"type": "request_header", "role": "host", "meta": map[string]any{"step": step}},
		})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)

		llmIn := llmCompleteIn{Messages: msgs, Tools: toolsJSON}
		stream := true
		llmIn.Stream = &stream

		type llmRes struct {
			out llmCompleteOut
			err error
		}
		ch := make(chan llmRes, 1)
		go func() {
			raw, err := p.sdk.CallWithCallback("llm", "complete", mustJSON(llmIn), func(ev *event) {
				if emit != nil {
					_ = emit(ev.Payload)
				}
			})
			if err != nil {
				ch <- llmRes{err: err}
				return
			}
			var cout llmCompleteOut
			_ = json.Unmarshal(raw, &cout)
			ch <- llmRes{out: cout}
		}()

		var llmOut llmCompleteOut
		select {
		case <-st.done:
			out.Cancelled = true
		case r := <-ch:
			if r.err != nil {
				return out, r.err
			}
			llmOut = r.out
		}
		if out.Cancelled {
			_, _ = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "turn_end", "role": "host", "meta": map[string]any{"reason": "cancelled"},
			}})
			out.Seqs = seqs
			return out, nil
		}

		out.Usage.add(llmOut.Usage)

		asst := map[string]any{
			"type": "message", "role": "assistant", "content": llmOut.Content,
		}
		if len(llmOut.ToolCalls) > 0 {
			asst["tool_calls"] = llmOut.ToolCalls
		}
		s, err = p.appendFacts(in.SessionID, []map[string]any{asst})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)
		out.Reply = llmOut.Content

		if len(llmOut.ToolCalls) == 0 {
			s, err = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "step_end", "role": "host", "meta": map[string]any{"reason": "completed", "step": step},
			}})
			seqs = append(seqs, s...)
			break
		}

		for _, tc := range llmOut.ToolCalls {
			if st.isCancelled() {
				out.Cancelled = true
				break
			}
			content, callErr := p.callTool(tc.Function.Name, tc.Function.Arguments, in.Workspace, tc.ID)
			if callErr != nil {
				content = "tool error: " + callErr.Error()
			}
			s, err = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "message", "role": "tool",
				"content": content, "tool_call_id": tc.ID,
				"meta": map[string]any{"tool_name": tc.Function.Name},
			}})
			if err != nil {
				return out, err
			}
			seqs = append(seqs, s...)
		}

		s, err = p.appendFacts(in.SessionID, []map[string]any{{
			"type": "step_end", "role": "host", "meta": map[string]any{"reason": "tool_calls", "step": step},
		}})
		seqs = append(seqs, s...)

		if step == maxSteps {
			s, err = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "turn_end", "role": "host", "meta": map[string]any{"reason": "max_steps"},
			}})
			seqs = append(seqs, s...)
			out.Seqs = seqs
			return out, nil
		}
	}

	if !out.Cancelled {
		reason := "completed"
		if out.Steps >= maxSteps {
			reason = "max_steps"
		}
		s, err = p.appendFacts(in.SessionID, []map[string]any{{
			"type": "turn_end", "role": "host", "meta": map[string]any{"reason": reason},
		}})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)
	}
	out.Seqs = seqs
	return out, nil
}
