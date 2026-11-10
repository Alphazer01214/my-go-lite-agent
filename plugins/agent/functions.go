package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) turn(ctx context.Context, r *pluginsdk.Request, in *fromWebUI) (resp *turnResp, err error) {
	input := strings.TrimSpace(in.UserInput)
	if input == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "user_input is required")
	}

	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sessionID, model, err := p.ensureSession(turnCtx, in, input)
	if err != nil {
		return nil, err
	}

	turnID := p.newTurnID()
	if err := p.registerTurn(sessionID, turnID, cancel); err != nil {
		return nil, err
	}
	defer p.unregisterTurn(sessionID, turnID)

	started, err := json.Marshal(loopStartedEvent{SessionId: sessionID, TurnId: turnID})
	if err != nil {
		return nil, err
	}
	_ = p.sdk.EmitWithID(r.ID, "loop", "started", started)

	startedAt := time.Now()
	startMeta, err := json.Marshal(turnStartMeta{TurnId: turnID, Workspace: in.Workspace, Model: model})
	if err != nil {
		return nil, err
	}
	if _, err := p.appendFacts(turnCtx, sessionID, []sessionFact{{
		Type: factTypeTurnStart,
		Role: "host",
		Meta: startMeta,
	}}); err != nil {
		return nil, err
	}
	defer func() {
		status := "ok"
		if err != nil {
			if isCancelled(err) {
				status = "cancelled"
			} else {
				status = "error"
			}
		}
		p.finishTurn(sessionID, turnID, status, err, time.Since(startedAt))
	}()

	userSeqs, err := p.appendFacts(turnCtx, sessionID, []sessionFact{{
		Type:        factTypeMessage,
		Role:        pluginsdk.RoleUser,
		TextContent: input,
	}})
	if err != nil {
		return nil, err
	}

	seqs := append([]int(nil), userSeqs...)
	modelMessages := make([]pluginsdk.ChatMessage, 0)
	maxSteps := p.currentConfig().MaxSteps
	for step := 1; step <= maxSteps; step++ {
		reply, err := p.callLLM(turnCtx, r.ID, sessionID, turnID, in.Workspace, model)
		if err != nil {
			return nil, err
		}
		if err := normalizeToolCalls(reply.Messages, turnID, step); err != nil {
			return nil, err
		}

		assistantSeqs, err := p.appendMessages(turnCtx, sessionID, reply.Messages)
		if err != nil {
			return nil, err
		}
		seqs = append(seqs, assistantSeqs...)
		usageSeqs, err := p.appendUsage(turnCtx, sessionID, turnID, step, reply)
		if err != nil {
			return nil, err
		}
		seqs = append(seqs, usageSeqs...)
		modelMessages = append(modelMessages, reply.Messages...)

		toolCalls := collectToolCalls(reply.Messages)
		if len(toolCalls) == 0 {
			return &turnResp{
				SessionId: sessionID,
				Model:     reply.Model,
				Messages:  modelMessages,
				Seqs:      seqs,
			}, nil
		}
		if step == maxSteps {
			return nil, pluginsdk.ErrCode(
				codeMaxStepsExceeded,
				fmt.Sprintf("agent reached max_steps=%d before the tool loop completed", maxSteps),
			)
		}

		for _, call := range toolCalls {
			result, err := p.executeToolCall(turnCtx, sessionID, in.Workspace, call)
			if err != nil {
				return nil, err
			}
			resultSeqs, err := p.appendFacts(turnCtx, sessionID, []sessionFact{result})
			if err != nil {
				return nil, err
			}
			seqs = append(seqs, resultSeqs...)
		}
	}

	return nil, pluginsdk.ErrCode(
		codeMaxStepsExceeded,
		fmt.Sprintf("agent reached max_steps=%d before the tool loop completed", maxSteps),
	)
}

func (p *plugin) ensureSession(ctx context.Context, in *fromWebUI, input string) (string, string, error) {
	id := strings.TrimSpace(in.SessionId)
	if id != "" {
		payload, err := json.Marshal(sessionInfoReq{SessionId: id})
		if err != nil {
			return "", "", err
		}
		raw, err := p.sdk.Call(ctx, "session", "info", payload)
		if err == nil {
			var info sessionInfoResp
			if err := json.Unmarshal(raw, &info); err != nil {
				return "", "", err
			}
			model := strings.TrimSpace(info.Model)
			if model == "" {
				model = strings.TrimSpace(in.Model)
			}
			return info.SessionId, model, nil
		}
		if pluginsdk.Code(err) != codeSessionNotFound {
			return "", "", err
		}
	}

	payload, err := json.Marshal(sessionCreateReq{
		Title:     titleFromInput(input),
		Workspace: in.Workspace,
		Origin:    "agent",
		Model:     strings.TrimSpace(in.Model),
	})
	if err != nil {
		return "", "", err
	}
	raw, err := p.sdk.Call(ctx, "session", "create", payload)
	if err != nil {
		return "", "", err
	}
	var meta sessionMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", "", err
	}
	return meta.SessionId, meta.Model, nil
}

func (p *plugin) appendFacts(ctx context.Context, sessionID string, facts []sessionFact) ([]int, error) {
	payload, err := json.Marshal(sessionAppendReq{SessionId: sessionID, Facts: facts})
	if err != nil {
		return nil, err
	}
	raw, err := p.sdk.Call(ctx, "session", "append", payload)
	if err != nil {
		return nil, err
	}
	var out sessionAppendResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Seqs, nil
}

func (p *plugin) callLLM(
	ctx context.Context,
	callerID, sessionID, turnID, workspace, model string,
) (*llmStreamResp, error) {
	payload, err := json.Marshal(llmStreamReq{
		Model: model,
		Meta: llmStreamMeta{
			SessionId: sessionID,
			TurnId:    turnID,
			Workspace: workspace,
			Model:     model,
		},
	})
	if err != nil {
		return nil, err
	}
	raw, err := p.sdk.CallWithCallback(ctx, "llm", "stream", payload, func(ev *pluginsdk.Event) {
		if ev.Method != "chunk" {
			return
		}
		_ = p.sdk.EmitWithID(callerID, "loop", "chunk", ev.Payload)
	})
	if err != nil {
		return nil, err
	}
	var out llmStreamResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *plugin) appendUsage(
	ctx context.Context,
	sessionID, turnID string,
	step int,
	reply *llmStreamResp,
) ([]int, error) {
	if reply == nil || reply.Usage == nil {
		return nil, nil
	}
	meta, err := json.Marshal(usageMeta{
		TurnId:           turnID,
		Step:             step,
		Model:            reply.Model,
		PromptTokens:     reply.Usage.PromptTokens,
		CompletionTokens: reply.Usage.CompletionTokens,
		TotalTokens:      reply.Usage.TotalTokens,
	})
	if err != nil {
		return nil, err
	}
	return p.appendFacts(ctx, sessionID, []sessionFact{{
		Type: factTypeUsage,
		Role: "host",
		Meta: meta,
	}})
}

func normalizeToolCalls(messages []pluginsdk.ChatMessage, turnID string, step int) error {
	for messageIndex := range messages {
		for callIndex := range messages[messageIndex].ToolCalls {
			call := &messages[messageIndex].ToolCalls[callIndex]
			call.ID = strings.TrimSpace(call.ID)
			if call.ID == "" {
				call.ID = fmt.Sprintf("%s-tool-%d-%d", turnID, step, callIndex+1)
			}
			call.Type = strings.TrimSpace(call.Type)
			if call.Type == "" {
				call.Type = "function"
			}
			call.FunctionCall.Name = strings.TrimSpace(call.FunctionCall.Name)
			if call.FunctionCall.Name == "" {
				return pluginsdk.ErrCode(codeToolCallInvalid, "tool call function name is required")
			}
			arguments := bytes.TrimSpace(call.FunctionCall.Arguments)
			if len(arguments) == 0 || bytes.Equal(arguments, []byte("null")) {
				arguments = json.RawMessage(`{}`)
			}
			if bytes.Equal(arguments[:1], []byte(`"`)) {
				var encoded string
				if err := json.Unmarshal(arguments, &encoded); err != nil {
					return pluginsdk.ErrCode(codeToolCallInvalid, "tool call arguments must be a JSON object")
				}
				arguments = json.RawMessage(encoded)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(arguments, &object); err != nil || object == nil {
				return pluginsdk.ErrCode(codeToolCallInvalid, "tool call arguments must be a JSON object")
			}
			call.FunctionCall.Arguments = arguments
		}
	}
	return nil
}

func collectToolCalls(messages []pluginsdk.ChatMessage) []pluginsdk.ToolCall {
	var calls []pluginsdk.ToolCall
	for _, message := range messages {
		calls = append(calls, message.ToolCalls...)
	}
	return calls
}

func (p *plugin) executeToolCall(
	ctx context.Context,
	sessionID, workspace string,
	call pluginsdk.ToolCall,
) (sessionFact, error) {
	payload, err := json.Marshal(toolCallReq{
		Name:      call.FunctionCall.Name,
		Arguments: call.FunctionCall.Arguments,
		SessionID: sessionID,
		Workspace: workspace,
		CallID:    call.ID,
	})
	if err != nil {
		return sessionFact{}, err
	}
	raw, err := p.sdk.Call(ctx, "tool", "call", payload)
	if err != nil {
		if isCancelled(err) || pluginsdk.Code(err) == pluginsdk.CodeCallDeadlineExceeded {
			return sessionFact{}, err
		}
		return toolErrorFact(call.ID, err), nil
	}

	var result toolCallResp
	if err := json.Unmarshal(raw, &result); err != nil {
		return toolErrorFact(call.ID, err), nil
	}
	content := strings.TrimSpace(result.Content)
	if content == "" {
		content = "(no output)"
	}
	return sessionFact{
		Type:        factTypeMessage,
		Role:        pluginsdk.RoleTool,
		TextContent: content,
		ToolCallID:  call.ID,
		Meta:        toolResultMeta(result),
	}, nil
}

func toolResultMeta(result toolCallResp) json.RawMessage {
	meta := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(result.Meta)) > 0 {
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(result.Meta, &extra); err == nil {
			for key, value := range extra {
				meta[key] = value
			}
		}
	}
	meta["is_error"], _ = json.Marshal(result.IsError)
	meta["truncated"], _ = json.Marshal(result.Truncated)
	encoded, err := json.Marshal(meta)
	if err != nil {
		return result.Meta
	}
	return encoded
}

func toolErrorFact(callID string, err error) sessionFact {
	code := pluginsdk.Code(err)
	if code == "" {
		code = pluginsdk.CodeHandlerError
	}
	meta, _ := json.Marshal(map[string]any{"error_code": code, "is_error": true, "truncated": false})
	return sessionFact{
		Type:        factTypeMessage,
		Role:        pluginsdk.RoleTool,
		TextContent: fmt.Sprintf("tool_error[%s]: %s", code, err.Error()),
		ToolCallID:  callID,
		Meta:        meta,
	}
}

func (p *plugin) appendMessages(ctx context.Context, sessionID string, messages []pluginsdk.ChatMessage) ([]int, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	facts := make([]sessionFact, 0, len(messages))
	for _, m := range messages {
		facts = append(facts, sessionFact{
			Type:        factTypeMessage,
			Role:        m.Role,
			TextContent: m.Text,
			Reasoning:   m.Reasoning,
			Files:       m.Files,
			ToolCalls:   m.ToolCalls,
			ToolCallID:  m.ToolCallID,
		})
	}
	return p.appendFacts(ctx, sessionID, facts)
}

func (p *plugin) cancel(ctx context.Context, req *cancelReq) (*cancelResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(req.SessionId)
	if sessionID == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "session_id is required")
	}
	p.mu.Lock()
	turn := p.turns[sessionID]
	p.mu.Unlock()
	if turn == nil {
		return &cancelResp{SessionId: sessionID, Cancelled: false}, nil
	}
	turn.cancel()
	return &cancelResp{SessionId: sessionID, Cancelled: true}, nil
}

func (p *plugin) newTurnID() string {
	return fmt.Sprintf("t-%d-%d", time.Now().UnixMilli(), p.seq.Add(1))
}

func (p *plugin) registerTurn(sessionID, turnID string, cancel context.CancelFunc) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.turns[sessionID] != nil {
		return pluginsdk.ErrCode(codeTurnBusy, "a turn is already running for session "+sessionID)
	}
	p.turns[sessionID] = &activeTurn{id: turnID, cancel: cancel}
	return nil
}

func (p *plugin) unregisterTurn(sessionID, turnID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if turn := p.turns[sessionID]; turn != nil && turn.id == turnID {
		delete(p.turns, sessionID)
	}
}

func (p *plugin) finishTurn(sessionID, turnID, status string, cause error, elapsed time.Duration) {
	meta := turnEndMeta{TurnId: turnID, Status: status, DurationMs: elapsed.Milliseconds()}
	if cause != nil {
		meta.Error = cause.Error()
		meta.ErrorCode = pluginsdk.Code(cause)
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = p.appendFacts(ctx, sessionID, []sessionFact{{
		Type: factTypeTurnEnd,
		Role: "host",
		Meta: raw,
	}})
}

func isCancelled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return pluginsdk.Code(err) == pluginsdk.CodeCallCancelled
}

func titleFromInput(input string) string {
	compact := strings.Join(strings.Fields(input), " ")
	if compact == "" {
		return "New Session"
	}
	runes := []rune(compact)
	if len(runes) > 30 {
		return string(runes[:30]) + "…"
	}
	return compact
}
