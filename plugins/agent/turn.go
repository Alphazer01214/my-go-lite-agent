package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type turnState struct {
	ctx          context.Context
	sessionID    string
	turnID       string
	stepID       string
	activeCallID string
	streamSeq    int64
	mu           sync.Mutex
	cancelled    bool
	done         chan struct{}
}

func newTurnState(ctx context.Context, id string) *turnState {
	return &turnState{ctx: ctx, sessionID: id, done: make(chan struct{})}
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

func (t *turnState) setTurnID(id string) {
	t.mu.Lock()
	t.turnID = id
	t.mu.Unlock()
}

func (t *turnState) setStepID(id string) {
	t.mu.Lock()
	t.stepID = id
	t.mu.Unlock()
}

func (t *turnState) beginCall(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled {
		return false
	}
	t.activeCallID = id
	return true
}

func (t *turnState) endCall(id string) {
	t.mu.Lock()
	if t.activeCallID == id {
		t.activeCallID = ""
	}
	t.mu.Unlock()
}

func (t *turnState) cancelContext() (callID, turnID, stepID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.activeCallID, t.turnID, t.stepID
}

func (t *turnState) withNextUpdate(snapshot streamSnapshot) streamSnapshot {
	snapshot.UpdateID = t.nextUpdateID()
	return snapshot
}
func (t *turnState) nextUpdateID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.streamSeq++
	return int(t.streamSeq)
}

type emitFunc func(payload json.RawMessage) error

// turnFinalizeTimeout bounds the terminal writes that must survive the handler
// context being canceled. It is short because these writes are local calls.
const turnFinalizeTimeout = 2 * time.Second

// finalizeContext detaches terminal bookkeeping from the caller's cancellation.
// A canceled inbound call must still be able to persist turn_end.
func finalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), turnFinalizeTimeout)
}

func (p *plugin) turn(ctx context.Context, in turnIn) (turnOut, error) {
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
		sessionID, err = p.createSession(ctx, in)
		if err != nil {
			return turnOut{}, err
		}
	}
	workspaceID, workspace, workspaceErr := p.sessionWorkspace(ctx, sessionID)
	if workspaceErr != nil {
		return turnOut{}, workspaceErr
	}
	if in.Workspace != "" && !sameWorkspacePath(in.Workspace, workspace) {
		return turnOut{}, errBadArguments("workspace does not match the session workspace")
	}
	if workspace == "" {
		return turnOut{}, errBadArguments("session workspace is required")
	}
	in.WorkspaceID = workspaceID
	in.Workspace = workspace

	slot := p.slotFor(sessionID)
	slot.turnMu.Lock()
	defer slot.turnMu.Unlock()

	st := newTurnState(ctx, sessionID)
	slot.mu.Lock()
	slot.active = st
	slot.mu.Unlock()
	defer func() {
		slot.mu.Lock()
		slot.active = nil
		slot.mu.Unlock()
	}()

	return p.runTurn(st, turnIn{
		Input:           in.Input,
		SessionID:       sessionID,
		WorkspaceID:     in.WorkspaceID,
		Workspace:       in.Workspace,
		MaxSteps:        maxSteps,
		AllowedTools:    in.AllowedTools,
		DeniedTools:     in.DeniedTools,
		ParentSessionID: in.ParentSessionID,
		Origin:          in.Origin,
		DelegationDepth: in.DelegationDepth,
	}, scheme, maxSteps)
}

func (p *plugin) cancel(ctx context.Context, in cancelIn) (cancelOut, error) {
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
	callID, turnID, stepID := active.cancelContext()
	if callID != "" {
		_, _ = p.sdk.Call(ctx, "llm", "cancel", mustJSON(llmCancelIn{CallID: callID}))
	}
	if turnID != "" && stepID != "" {
		_ = p.updateStream(ctx, streamSnapshot{
			SessionID: in.SessionID,
			TurnID:    turnID,
			StepID:    stepID,
			UpdateID:  active.nextUpdateID(),
			Message:   streamMessage{Role: RoleAssistant},
			Status:    "cancel_requested",
		})
	}
	return cancelOut{SessionID: in.SessionID, Cancelled: true}, nil
}

func (p *plugin) createSession(ctx context.Context, in turnIn) (string, error) {
	raw, err := p.sdk.Call(ctx, "session", "create", mustJSON(sessionCreateIn{
		WorkspaceID: in.WorkspaceID, Workspace: in.Workspace, ParentSessionID: in.ParentSessionID,
		Origin: in.Origin, DelegationDepth: in.DelegationDepth,
	}))
	if err != nil {
		return "", err
	}
	var out sessionCreateOut
	if err := json.Unmarshal(raw, &out); err != nil || out.SessionID == "" {
		return "", errHandler("bad session.create response")
	}
	return out.SessionID, nil
}

func (p *plugin) sessionWorkspace(ctx context.Context, sessionID string) (string, string, error) {
	raw, err := p.sdk.Call(ctx, "session", "info", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return "", "", err
	}
	var out sessionInfoOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", errHandler("bad session.info response")
	}
	return out.WorkspaceID, out.Workspace, nil
}

func (p *plugin) appendFacts(ctx context.Context, sessionID string, facts []map[string]any) ([]int, error) {
	raw, err := p.sdk.Call(ctx, "session", "append", mustJSON(sessionAppendIn{SessionID: sessionID, Facts: facts}))
	if err != nil {
		return nil, err
	}
	var out sessionAppendOut
	_ = json.Unmarshal(raw, &out)
	return out.Seqs, nil
}

func (p *plugin) assembleMessages(ctx context.Context, sessionID string) ([]chatMessage, error) {
	raw, err := p.sdk.Call(ctx, "session", "derive", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return nil, err
	}
	var out messagesOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errHandler("bad session.derive response")
	}
	return out.Messages, nil
}

// buildModelContext asks model-context for the prompt. A missing or failing
// capability degrades to session.derive + tools.list: the turn still runs, but
// the caller is told so it does not report a token estimate that was never made.
func (p *plugin) buildModelContext(ctx context.Context, in turnIn, scheme, turnID, stepID string) (modelContextOut, error) {
	payload := mustJSON(modelContextIn{
		SessionID: in.SessionID, TurnID: turnID, StepID: stepID, Scheme: scheme,
		WorkspaceID: in.WorkspaceID, Workspace: in.Workspace,
		AllowedTools: in.AllowedTools, DeniedTools: in.DeniedTools,
	})
	raw, err := p.sdk.Call(ctx, "model-context", "build", payload)
	degraded := ""
	if err == nil {
		var out modelContextOut
		decodeErr := json.Unmarshal(raw, &out)
		if decodeErr == nil {
			return out, nil
		}
		degraded = "bad model-context response: " + decodeErr.Error()
	} else {
		degraded = "model-context unavailable: " + err.Error()
	}
	messages, err := p.assembleMessages(ctx, in.SessionID)
	if err != nil {
		return modelContextOut{}, err
	}
	_, tools, toolsDegraded, toolErr := p.collectTools(ctx, scheme, in.AllowedTools, in.DeniedTools)
	if toolErr != nil {
		return modelContextOut{}, toolErr
	}
	if toolsDegraded != "" {
		if degraded != "" {
			degraded += "; "
		}
		degraded += toolsDegraded
	}
	return modelContextOut{Messages: messages, Tools: tools, Degraded: degraded}, nil
}

// collectTools resolves the tool list for this scheme. tools.list is optional,
// so a failure degrades to "no tools" — never to a failed turn — and the reason
// is reported through degraded so the caller can log it.
func (p *plugin) collectTools(ctx context.Context, scheme string, allowed, denied []string) ([]toolSpec, []toolDefinition, string, error) {
	degraded := ""
	if len(allowed) == 0 && scheme == schemeChat {
		return nil, nil, "", nil
	}
	raw, err := p.sdk.Call(ctx, "tools", "list", mustJSON(map[string]any{}))
	if err != nil {
		return nil, nil, "tools unavailable: " + err.Error(), nil
	}
	var listed struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return nil, nil, "bad tools.list response: " + err.Error(), nil
	}
	filtered := filterTools(listed.Tools, scheme, allowed)
	if len(denied) > 0 {
		blocked := make(map[string]bool, len(denied))
		for _, name := range denied {
			blocked[name] = true
		}
		kept := filtered[:0]
		for _, tool := range filtered {
			if !blocked[tool.Name] {
				kept = append(kept, tool)
			}
		}
		filtered = kept
	}
	if len(filtered) == 0 {
		return nil, nil, degraded, nil
	}
	tools := make([]toolDefinition, 0, len(filtered))
	for _, t := range filtered {
		params := t.InputSchema
		if len(t.InputSchema) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		description := t.Description
		if t.Risk != "" {
			description += "\nRisk: " + t.Risk + "."
		}
		tools = append(tools, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name:        t.Name,
				Description: description,
				Parameters:  params,
			},
		})
	}
	return filtered, tools, degraded, nil
}

func (p *plugin) callTool(ctx context.Context, name, arguments, sessionID, workspace, callID string) (toolResult, error) {
	args := json.RawMessage(arguments)
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	raw, err := p.sdk.Call(ctx, "tools", "call", mustJSON(map[string]any{
		"name":       name,
		"arguments":  args,
		"session_id": sessionID,
		"workspace":  workspace,
		"call_id":    callID,
	}))
	if err != nil {
		return toolResult{}, err
	}
	var out toolResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return toolResult{}, errHandler("bad tools.call result: " + err.Error())
	}
	return out, nil
}

func (p *plugin) runTurn(st *turnState, in turnIn, scheme string, maxSteps int) (out turnOut, retErr error) {
	out = turnOut{SessionID: in.SessionID}
	seqs := make([]int, 0, 16)
	turnID := fmt.Sprintf("t-%d-%d", time.Now().UnixNano(), p.turnSeq.Add(1))
	st.setTurnID(turnID)

	appendMeta := func(stepID string, extra map[string]any) map[string]any {
		meta := map[string]any{"turn_id": turnID, "plugin": "agent"}
		if stepID != "" {
			meta["step_id"] = stepID
		}
		for key, value := range extra {
			meta[key] = value
		}
		return meta
	}

	turnStarted := false
	turnEnded := false
	writeTurnEnd := func(ctx context.Context, reason string, extra map[string]any) error {
		if turnEnded {
			return nil
		}
		meta := appendMeta("", map[string]any{"reason": reason})
		for key, value := range extra {
			meta[key] = value
		}
		s, err := p.appendFacts(ctx, in.SessionID, []map[string]any{{
			"type": "turn_end", "role": "host", "meta": meta,
		}})
		if err != nil {
			return err
		}
		turnEnded = true
		seqs = append(seqs, s...)
		out.Seqs = seqs
		return nil
	}

	defer func() {
		if retErr == nil || !turnStarted || turnEnded {
			return
		}
		reason := "error"
		extra := map[string]any{"error": retErr.Error()}
		if st.isCancelled() || st.ctx.Err() != nil {
			reason = "cancelled"
			extra = nil
			out.Cancelled = true
		}
		ctx, cancel := finalizeContext(st.ctx)
		defer cancel()
		_ = writeTurnEnd(ctx, reason, extra)
		out.Seqs = seqs
	}()

	s, err := p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
		"type": "turn_start", "role": "host", "meta": appendMeta("", map[string]any{
			"workspace_id": in.WorkspaceID, "workspace": in.Workspace,
		}),
	}})
	if err != nil {
		return out, err
	}
	turnStarted = true
	seqs = append(seqs, s...)

	s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
		"type": "message", "role": "user", "content": in.Input, "meta": appendMeta("", nil),
	}})
	if err != nil {
		return out, err
	}
	seqs = append(seqs, s...)

	// A turn that exits because the step budget ran out must not be recorded as
	// "completed"; one that finished its last step normally is.
	lastStepHadToolCalls := false
	for step := 1; step <= maxSteps; step++ {
		if st.isCancelled() || st.ctx.Err() != nil {
			if st.ctx.Err() != nil {
				st.markCancelled()
			}
			out.Cancelled = true
			break
		}
		out.Steps = step
		stepID := fmt.Sprintf("%s-s%d", turnID, step)
		st.setStepID(stepID)

		contextOut, contextErr := p.buildModelContext(st.ctx, in, scheme, turnID, stepID)
		if contextErr != nil {
			return out, contextErr
		}
		msgs, tools := contextOut.Messages, contextOut.Tools

		headerMeta := map[string]any{
			"step": step, "model_context_id": contextOut.ModelContextID,
			"token_estimate": contextOut.TokenEstimate,
		}
		if contextOut.Degraded != "" {
			headerMeta["degraded"] = contextOut.Degraded
		}
		s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{
			{"type": "step_start", "role": "host", "meta": appendMeta(stepID, map[string]any{"step": step})},
			{"type": "request_header", "role": "host", "meta": appendMeta(stepID, headerMeta)},
		})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)

		callID := fmt.Sprintf("%s-s%d", turnID, step)
		if !st.beginCall(callID) {
			out.Cancelled = true
			break
		}
		llmIn := llmCompleteIn{CallID: callID, Messages: msgs, Tools: tools}
		stream := true
		llmIn.Stream = &stream

		type llmRes struct {
			out llmCompleteOut
			err error
		}
		events := make(chan json.RawMessage, 128)
		streamDone := make(chan struct{})
		stopStream := make(chan struct{})
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
						_ = p.updateStream(st.ctx, st.withNextUpdate(acc.snapshot(in.SessionID, turnID, stepID, "streaming")))
					}
				case <-stopStream:
					return
				}
			}
		}()

		result := make(chan llmRes, 1)
		go func() {
			raw, err := p.sdk.CallWithCallback(st.ctx, "llm", "complete", mustJSON(llmIn), func(ev *event) {
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
			if decodeErr := json.Unmarshal(raw, &cout); decodeErr != nil {
				result <- llmRes{err: errHandler("bad llm.complete response: " + decodeErr.Error())}
				return
			}
			result <- llmRes{out: cout}
		}()

		var llmOut llmCompleteOut
		cancelWait := func() {
			out.Cancelled = true
			close(stopStream)
			<-streamDone
		}
		select {
		case <-st.done:
			cancelWait()
		case <-st.ctx.Done():
			st.markCancelled()
			cancelWait()
		case r := <-result:
			if r.err != nil {
				<-streamDone
				if st.ctx.Err() == nil && !st.isCancelled() {
					return out, r.err
				}
				if st.ctx.Err() != nil {
					st.markCancelled()
				}
				out.Cancelled = true
				break
			}
			llmOut = r.out
			<-streamDone
		}
		if out.Cancelled {
			finCtx, finCancel := finalizeContext(st.ctx)
			partial := acc.message()
			out.Reply = partial.Content
			if partial.Content != "" || partial.Reasoning != "" || len(partial.ToolCalls) > 0 {
				_ = p.updateStream(finCtx, st.withNextUpdate(streamSnapshot{
					SessionID: in.SessionID,
					TurnID:    turnID,
					StepID:    stepID,
					Message:   partial,
					Status:    "cancelled",
				}))
				s, err = p.appendFacts(finCtx, in.SessionID, []map[string]any{{
					"type": "message", "role": "assistant", "content": partial.Content,
					"meta": appendMeta(stepID, map[string]any{"reasoning": partial.Reasoning}),
				}})
				if err == nil {
					seqs = append(seqs, s...)
				}
			}
			err = writeTurnEnd(finCtx, "cancelled", nil)
			finCancel()
			if err != nil {
				return out, err
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
		_ = p.updateStream(st.ctx, st.withNextUpdate(streamSnapshot{
			SessionID: in.SessionID,
			TurnID:    turnID,
			StepID:    stepID,
			Message:   final,
			Status:    "complete",
		}))
		addUsage(&out.Usage, llmOut.Usage)

		s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{
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
			s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
				"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{"reason": "completed", "step": step}),
			}})
			if err != nil {
				return out, err
			}
			seqs = append(seqs, s...)
			break
		}

		cancelledMidTools := false
		for _, tc := range llmOut.ToolCalls {
			if st.isCancelled() || st.ctx.Err() != nil {
				if st.ctx.Err() != nil {
					st.markCancelled()
				}
				out.Cancelled = true
				cancelledMidTools = true
				break
			}
			result, callErr := p.callTool(st.ctx, tc.Function.Name, tc.Function.Arguments, in.SessionID, in.Workspace, tc.ID)
			content := result.Content
			toolMeta := map[string]any{
				"tool_name": tc.Function.Name, "is_error": result.IsError,
				"truncated": result.Truncated,
			}
			if result.Meta != nil {
				toolMeta["tool_meta"] = result.Meta
			}
			if callErr != nil {
				content = "tool error: " + callErr.Error()
				toolMeta["is_error"] = true
				toolMeta["wire_error"] = callErr.Error()
			}
			s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
				"type": "message", "role": "tool", "content": content, "tool_call_id": tc.ID,
				"meta": appendMeta(stepID, toolMeta),
			}})
			if err != nil {
				return out, err
			}
			seqs = append(seqs, s...)
		}
		if cancelledMidTools {
			// The assistant fact above already lists every tool_call of this step,
			// but only the ones executed got a tool result. Closing the step as
			// "tool_calls" would leave dangling tool_call_ids in the transcript,
			// so the step is closed as cancelled and the turn ends here.
			finCtx, finCancel := finalizeContext(st.ctx)
			s, err = p.appendFacts(finCtx, in.SessionID, []map[string]any{{
				"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{"reason": "cancelled", "step": step}),
			}})
			finCancel()
			if err != nil {
				return out, err
			}
			seqs = append(seqs, s...)
			break
		}

		s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
			"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{"reason": "tool_calls", "step": step}),
		}})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)
		lastStepHadToolCalls = true
	}

	if out.Cancelled {
		finCtx, finCancel := finalizeContext(st.ctx)
		err = writeTurnEnd(finCtx, "cancelled", nil)
		finCancel()
		if err != nil {
			return out, err
		}
	} else {
		reason := "completed"
		if out.Steps >= maxSteps && lastStepHadToolCalls {
			// Only a step that still wanted more work is capped; a step that
			// finished on its own is "completed" even on the last allowed step.
			reason = "max_steps"
		}
		finCtx, finCancel := finalizeContext(st.ctx)
		err = writeTurnEnd(finCtx, reason, nil)
		finCancel()
		if err != nil {
			return out, err
		}
	}
	out.Seqs = seqs
	return out, nil
}
