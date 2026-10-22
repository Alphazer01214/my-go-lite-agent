package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"strings"
	"time"
)

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
	info, infoErr := p.sessionInfo(ctx, sessionID)
	if infoErr != nil {
		return turnOut{}, infoErr
	}
	if in.Workspace != "" && !sameWorkspacePath(in.Workspace, info.Workspace) {
		return turnOut{}, errBadArguments("workspace does not match the session workspace")
	}
	if info.Workspace == "" {
		return turnOut{}, errBadArguments("session workspace is required")
	}
	in.WorkspaceID = info.WorkspaceID
	in.Workspace = info.Workspace

	// The session records the model chosen at creation. A catalog edit may have
	// removed that id since then; resolve it before the first request so a stale
	// name does not reach the upstream.
	model, contextWindow, fallbackFrom := p.resolveModel(ctx, info.Model, info.ContextWindow)

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
		Model:           model,
		ContextWindow:   contextWindow,
		MaxSteps:        maxSteps,
		AllowedTools:    in.AllowedTools,
		DeniedTools:     in.DeniedTools,
		ParentSessionID: in.ParentSessionID,
		Origin:          in.Origin,
		DelegationDepth: in.DelegationDepth,
	}, scheme, maxSteps, fallbackFrom)
}

func (p *plugin) runTurn(st *turnState, in turnIn, scheme string, maxSteps int, modelFallbackFrom string) (out turnOut, retErr error) {
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
		facts := []map[string]any{{
			"type": "turn_end", "role": "host", "meta": meta,
		}}
		if reason == "cancelled" {
			facts = append(facts, map[string]any{
				"type":    "message",
				"role":    "user",
				"content": cancelledTurnNote,
				"meta":    appendMeta("", map[string]any{"kind": "turn_cancelled"}),
			})
		}
		s, err := p.appendFacts(ctx, in.SessionID, facts)
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
		if code := pluginsdk.Code(retErr); code != "" {
			extra["error_code"] = code
		}
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
	toolCallCount := 0
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

		contextOut, contextErr := p.buildModelContext(st.ctx, in, scheme)
		if contextErr != nil {
			return out, contextErr
		}
		msgs, tools := contextOut.Messages, contextOut.Tools
		specs := contextOut.ToolSpecs

		headerMeta := map[string]any{
			"step": step, "mode": scheme, "scheme": scheme,
			"token_estimate": contextOut.TokenEstimate,
		}
		if in.Model != "" {
			headerMeta["model"] = in.Model
		}
		if in.ContextWindow > 0 {
			headerMeta["context_window"] = in.ContextWindow
		}
		if modelFallbackFrom != "" {
			headerMeta["model_fallback_from"] = modelFallbackFrom
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
		llmIn := llmCompleteIn{
			CallID: callID, Messages: msgs, Tools: tools, Model: in.Model,
			Meta: &llmMeta{
				SessionID: in.SessionID, TurnID: turnID, StepID: stepID,
				Mode: scheme, Scheme: scheme,
				WorkspaceID: in.WorkspaceID, Workspace: in.Workspace,
				Model: in.Model, ContextWindow: in.ContextWindow,
			},
		}
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

		assistantFact := map[string]any{
			"type": "message", "role": "assistant", "content": llmOut.Content,
			"meta": appendMeta(stepID, map[string]any{"reasoning": llmOut.Reasoning}),
		}
		// The provider rejects role=tool unless the preceding assistant message
		// carries the matching tool_calls. Persist them with the assistant fact
		// so the next derive is a valid request history.
		if len(llmOut.ToolCalls) > 0 {
			assistantFact["tool_calls"] = llmOut.ToolCalls
		}
		s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{
			{"type": "llm_usage", "role": "host", "meta": appendMeta(stepID, map[string]any{
				"usage": llmOut.Usage, "model": llmOut.Model, "context_window": llmOut.ContextWindow,
				"finish_reason": llmOut.FinishReason,
			})},
			assistantFact,
		})
		if err != nil {
			return out, err
		}
		seqs = append(seqs, s...)
		out.Reply = llmOut.Content

		if len(llmOut.ToolCalls) == 0 {
			finishReason := strings.TrimSpace(llmOut.FinishReason)
			switch finishReason {
			case "length":
				s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
					"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{
						"reason": "error", "step": step, "error_code": "llm_output_truncated",
					}),
				}})
				if err != nil {
					return out, err
				}
				seqs = append(seqs, s...)
				return out, errLlmOutputTruncated("model output hit the token limit before finishing")
			case "content_filter":
				s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
					"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{
						"reason": "error", "step": step, "error_code": "llm_content_filtered",
					}),
				}})
				if err != nil {
					return out, err
				}
				seqs = append(seqs, s...)
				return out, errLlmContentFiltered("model output was stopped by the upstream content filter")
			case "tool_calls", "function_call":
				s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
					"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{
						"reason": "error", "step": step, "error_code": "llm_tool_calls_missing",
					}),
				}})
				if err != nil {
					return out, err
				}
				seqs = append(seqs, s...)
				return out, errLlmToolCallsMissing("model reported tool_calls but returned none")
			}
			s, err = p.appendFacts(st.ctx, in.SessionID, []map[string]any{{
				"type": "step_end", "role": "host", "meta": appendMeta(stepID, map[string]any{
					"reason": "completed", "step": step, "finish_reason": finishReason,
					"scheme": scheme, "tool_calls": 0,
				}),
			}})
			if err != nil {
				return out, err
			}
			seqs = append(seqs, s...)
			break
		}

		toolCallCount += len(llmOut.ToolCalls)
		cancelledMidTools := false
		// Every tool_call in the persisted assistant fact needs a matching tool
		// result before the next request. When cancellation stops the loop, close
		// the remaining ids with an explicit "not executed" result instead of
		// leaving a transcript the provider rejects.
		appendCancelledToolResults := func(from int) error {
			facts := make([]map[string]any, 0, len(llmOut.ToolCalls)-from)
			for _, tc := range llmOut.ToolCalls[from:] {
				facts = append(facts, map[string]any{
					"type": "message", "role": "tool", "content": cancelledToolResult,
					"tool_call_id": tc.ID,
					"meta": appendMeta(stepID, map[string]any{
						"tool_name": tc.Function.Name, "is_error": true, "cancelled": true,
					}),
				})
			}
			if len(facts) == 0 {
				return nil
			}
			finCtx, finCancel := finalizeContext(st.ctx)
			defer finCancel()
			appended, err := p.appendFacts(finCtx, in.SessionID, facts)
			if err != nil {
				return err
			}
			seqs = append(seqs, appended...)
			return nil
		}
		if !st.isCancelled() && st.ctx.Err() == nil && p.canRunToolsParallel(specs, llmOut.ToolCalls) {
			cancelled, parallelSeqs, err := p.runToolCallsParallel(st, in, scheme, llmOut.ToolCalls)
			if err != nil {
				return out, err
			}
			seqs = append(seqs, parallelSeqs...)
			if cancelled {
				out.Cancelled = true
				cancelledMidTools = true
			}
		} else {
			for index, tc := range llmOut.ToolCalls {
				if st.isCancelled() || st.ctx.Err() != nil {
					if st.ctx.Err() != nil {
						st.markCancelled()
					}
					out.Cancelled = true
					cancelledMidTools = true
					if err := appendCancelledToolResults(index); err != nil {
						return out, err
					}
					break
				}
				result, callErr := p.callTool(st.ctx, tc.Function.Name, tc.Function.Arguments,
					in.SessionID, in.Workspace, in.WorkspaceID, scheme, tc.ID)
				content := result.Content
				toolMeta := map[string]any{
					"tool_name": tc.Function.Name, "is_error": result.IsError,
					"truncated": result.Truncated,
				}
				if result.Meta != nil {
					toolMeta["tool_meta"] = result.Meta
				}
				if callErr != nil {
					errCode := pluginsdk.Code(callErr)
					if errCode != "" {
						content = fmt.Sprintf("tool error [%s]: %s", errCode, callErr.Error())
						toolMeta["error_code"] = errCode
					} else {
						content = "tool error: " + callErr.Error()
					}
					toolMeta["is_error"] = true
					toolMeta["wire_error"] = callErr.Error()
				}
				appendCtx := st.ctx
				cancelled := st.isCancelled() || st.ctx.Err() != nil
				var releaseAppend context.CancelFunc
				if cancelled {
					appendCtx, releaseAppend = finalizeContext(st.ctx)
				}
				s, err = p.appendFacts(appendCtx, in.SessionID, []map[string]any{{
					"type": "message", "role": "tool", "content": content, "tool_call_id": tc.ID,
					"meta": appendMeta(stepID, toolMeta),
				}})
				if releaseAppend != nil {
					releaseAppend()
				}
				if err != nil {
					return out, err
				}
				seqs = append(seqs, s...)
				if cancelled {
					out.Cancelled = true
					cancelledMidTools = true
					if err := appendCancelledToolResults(index + 1); err != nil {
						return out, err
					}
					break
				}
			}
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
		err = writeTurnEnd(finCtx, reason, map[string]any{
			"tool_calls": toolCallCount, "steps": out.Steps,
		})
		finCancel()
		if err != nil {
			return out, err
		}
	}
	out.Seqs = seqs
	return out, nil
}
