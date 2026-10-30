package main

import (
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

	messages, err := p.sessionMessages(turnCtx, sessionID)
	if err != nil {
		return nil, err
	}

	reply, err := p.callLLM(turnCtx, r.ID, sessionID, turnID, in.Workspace, model, messages)
	if err != nil {
		return nil, err
	}

	assistantSeqs, err := p.appendMessages(turnCtx, sessionID, reply.Messages)
	if err != nil {
		return nil, err
	}

	return &turnResp{
		SessionId: sessionID,
		Model:     reply.Model,
		Messages:  reply.Messages,
		Seqs:      append(userSeqs, assistantSeqs...),
	}, nil
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

func (p *plugin) sessionMessages(ctx context.Context, sessionID string) ([]pluginsdk.ChatMessage, error) {
	payload, err := json.Marshal(sessionInfoReq{SessionId: sessionID})
	if err != nil {
		return nil, err
	}
	raw, err := p.sdk.Call(ctx, "session", "messages", payload)
	if err != nil {
		return nil, err
	}
	var out sessionMessagesResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func (p *plugin) callLLM(
	ctx context.Context,
	callerID, sessionID, turnID, workspace, model string,
	messages []pluginsdk.ChatMessage,
) (*llmStreamResp, error) {
	payload, err := json.Marshal(llmStreamReq{
		Model:    model,
		Messages: messages,
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
