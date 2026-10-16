package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
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

func (p *plugin) turn(in turnIn) (turnOut, error) {
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

	return p.runTurn(st, turnIn{
		Input:        in.Input,
		SessionID:    sessionID,
		Workspace:    in.Workspace,
		MaxSteps:     maxSteps,
		AllowedTools: in.AllowedTools,
	}, scheme, maxSteps)
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
	raw, err := p.sdk.Call("session", "derive", mustJSON(map[string]string{"session_id": sessionID}))
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

func (p *plugin) runTurn(st *turnState, in turnIn, scheme string, maxSteps int) (turnOut, error) {
	out := turnOut{SessionID: in.SessionID}
	seqs := make([]int, 0, 16)
	turnID := fmt.Sprintf("t-%d", time.Now().UnixNano())

	appendMeta := func(stepID string, extra map[string]any) map[string]any {
		meta := map[string]any{"turn_id": turnID}
		if stepID != "" {
			meta["step_id"] = stepID
		}
		for key, value := range extra {
			meta[key] = value
		}
		return meta
	}

	s, err := p.appendFacts(in.SessionID, []map[string]any{{
		"type": "turn_start", "role": "host", "meta": appendMeta("", map[string]any{"workspace": in.Workspace}),
	}})
	if err != nil {
		return out, err
	}
	seqs = append(seqs, s...)

	s, err = p.appendFacts(in.SessionID, []map[string]any{{
		"type": "message", "role": "user", "content": in.Input, "meta": appendMeta("", nil),
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
		stepID := fmt.Sprintf("%s-s%d", turnID, step)

		msgs, err := p.assembleMessages(in.SessionID)
		if err != nil {
			return out, err
		}

		s, err = p.appendFacts(in.SessionID, []map[string]any{
			{"type": "step_start", "role": "host", "meta": appendMeta(stepID, map[string]any{"step": step})},
			{"type": "request_header", "role": "host", "meta": appendMeta(stepID, map[string]any{"step": step})},
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
		events := make(chan json.RawMessage, 128)
		streamDone := make(chan struct{})
		stopStream := make(chan struct{})
		sequencer := newStreamSequencer()
		flushPolicy := newStreamFlushPolicy()
		acc := newStreamAccumulator()
		go func() {
			defer close(streamDone)
			for {
				select {
				case _, ok := <-events:
					if !ok {
						return
					}
					if flushPolicy.observe(time.Now()) {
						_ = p.updateStream(sequencer.withNext(acc.snapshot(in.SessionID, turnID, stepID, "streaming")))
					}
				case <-stopStream:
					return
				}
			}
		}()

		result := make(chan llmRes, 1)
		go func() {
			raw, err := p.sdk.CallWithCallback("llm", "complete", mustJSON(llmIn), func(ev *event) {
				if !acc.apply(ev.Payload) {
					return
				}
				select {
				case events <- ev.Payload:
				default:
				}
			})
			close(events)
			if err != nil {
				result <- llmRes{err: err}
				return
			}
			var cout llmCompleteOut
			_ = json.Unmarshal(raw, &cout)
			result <- llmRes{out: cout}
		}()

		var llmOut llmCompleteOut
		select {
		case <-st.done:
			out.Cancelled = true
			close(stopStream)
			<-streamDone
		case r := <-result:
			if r.err != nil {
				<-streamDone
				return out, r.err
			}
			llmOut = r.out
			<-streamDone
		}
		if out.Cancelled {
			partial := acc.message()
			out.Reply = partial.Content
			if partial.Content != "" || partial.Reasoning != "" || len(partial.ToolCalls) > 0 {
				_ = p.updateStream(sequencer.withNext(streamSnapshot{
					SessionID: in.SessionID,
					TurnID:    turnID,
					StepID:    stepID,
					Message:   partial,
					Status:    "cancelled",
				}))
				s, err = p.appendFacts(in.SessionID, []map[string]any{{
					"type": "message", "role": "assistant", "content": partial.Content,
					"meta": appendMeta(stepID, map[string]any{"reasoning": partial.Reasoning}),
				}})
				if err == nil {
					seqs = append(seqs, s...)
				}
			}
			s, err = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "turn_end", "role": "host", "meta": appendMeta("", map[string]any{"reason": "cancelled"}),
			}})
			if err == nil {
				seqs = append(seqs, s...)
			}
			out.Seqs = seqs
			return out, nil
		}

		final := streamMessage{
			Role:      RoleAssistant,
			Content:   llmOut.Content,
			Reasoning: llmOut.Reasoning,
			ToolCalls: llmOut.ToolCalls,
		}
		_ = p.updateStream(sequencer.withNext(streamSnapshot{
			SessionID: in.SessionID,
			TurnID:    turnID,
			StepID:    stepID,
			Message:   final,
			Status:    "complete",
		}))
		out.Usage.add(llmOut.Usage)

		s, err = p.appendFacts(in.SessionID, []map[string]any{
			{"type": "llm_usage", "role": "host", "meta": appendMeta(stepID, map[string]any{"usage": llmOut.Usage})},
			{
				"type": "message", "role": "assistant", "content": llmOut.Content,
				"meta": appendMeta(stepID, map[string]any{"reasoning": llmOut.Reasoning}),
			},
		})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)
		out.Reply = llmOut.Content

		if len(llmOut.ToolCalls) == 0 {
			s, err = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{"reason": "completed", "step": step}),
			}})
			if err != nil {
				return out, err
			}
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
				"type": "message", "role": "tool", "content": content, "tool_call_id": tc.ID,
				"meta": appendMeta(stepID, map[string]any{"tool_name": tc.Function.Name}),
			}})
			if err != nil {
				return out, err
			}
			seqs = append(seqs, s...)
		}

		s, err = p.appendFacts(in.SessionID, []map[string]any{{
			"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{"reason": "tool_calls", "step": step}),
		}})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)

		if step == maxSteps {
			s, err = p.appendFacts(in.SessionID, []map[string]any{{
				"type": "turn_end", "role": "host", "meta": appendMeta("", map[string]any{"reason": "max_steps"}),
			}})
			if err != nil {
				return out, err
			}
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
			"type": "turn_end", "role": "host", "meta": appendMeta("", map[string]any{"reason": reason}),
		}})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)
	}
	out.Seqs = seqs
	return out, nil
}
