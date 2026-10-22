package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// awaitConfirmation blocks the seam until the user approves, rejects or the
// token expires. The handler goroutine is the only place that waits; the SDK
// keeps serving sandbox.decide and the UI while it is parked here.
func (p *plugin) awaitConfirmation(
	ctx context.Context,
	req *pluginsdk.Request,
	payload seamPayload,
	verdict decision,
	call callContext,
	proceed seamProceed,
) (json.RawMessage, error) {
	p.mu.RLock()
	ttl := p.cfg.ConfirmTTLSeconds
	p.mu.RUnlock()
	if ttl <= 0 {
		ttl = 300
	}
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttl) * time.Second).UnixMilli()

	args, err := Arguments(payload.Arguments)
	if err != nil {
		return nil, err
	}
	item := &pendingConfirm{
		Token:            newConfirmToken(),
		Tool:             verdict.Tool,
		Capability:       verdict.Capability,
		Method:           verdict.Method,
		SessionID:        call.SessionID,
		RuleID:           verdict.RuleID,
		Reason:           verdict.Reason,
		Risk:             verdict.Risk,
		ArgumentsPreview: argumentsPreview(call, args),
		ArgumentsDigest:  Digest(payload.Arguments),
		CreatedTS:        now.UnixMilli(),
		ExpiresTS:        expiresAt,
		Decision:         make(chan string, 1),
	}
	p.pending.add(item)
	p.publishPending()
	p.writeAudit(ctx, verdict, call, payload.Arguments, item.Token, item.ExpiresTS)

	timer := time.NewTimer(time.Until(time.UnixMilli(item.ExpiresTS)))
	defer timer.Stop()
	select {
	case decision := <-item.Decision:
		p.publishPending()
		if ctx.Err() != nil {
			// The decision and a cancellation can land together. A cancelled
			// turn must not execute the call, even if the user pressed allow.
			cancelled := verdict
			cancelled.Action = "cancelled"
			cancelled.Reason = "call cancelled while awaiting confirmation"
			p.writeAudit(context.WithoutCancel(ctx), cancelled, call, payload.Arguments, item.Token, 0)
			return nil, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled,
				"call cancelled while awaiting confirmation")
		}
		if decision == ActionAllow {
			approved := verdict
			approved.Action = ActionAllow
			approved.Reason = "user approved the call: " + verdict.Reason
			p.writeAudit(context.WithoutCancel(ctx), approved, call, payload.Arguments, item.Token, 0)
			p.rememberApproved(req, call)
			return proceed(ctx, req.Payload)
		}
		rejected := verdict
		rejected.Action = ActionDeny
		rejected.Reason = "user rejected the call: " + verdict.Reason
		p.writeAudit(context.WithoutCancel(ctx), rejected, call, payload.Arguments, item.Token, 0)
		return denyResult(rejected)
	case <-timer.C:
		p.pending.remove(item.Token)
		p.publishPending()
		expired := verdict
		expired.Action = ActionDeny
		expired.Reason = fmt.Sprintf("confirmation expired after %d seconds", ttl)
		p.writeAudit(context.WithoutCancel(ctx), expired, call, payload.Arguments, item.Token, 0)
		return confirmExpiredResult(verdict, item.ExpiresTS)
	case <-ctx.Done():
		p.pending.remove(item.Token)
		p.publishPending()
		cancelled := verdict
		cancelled.Action = "cancelled"
		cancelled.Reason = "call cancelled while awaiting confirmation"
		p.writeAudit(context.WithoutCancel(ctx), cancelled, call, payload.Arguments, item.Token, 0)
		return nil, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled,
			"call cancelled while awaiting confirmation")
	}
}

// argumentsPreview gives the confirmation panel enough context without
// copying file contents or other values into the UI state.
func argumentsPreview(call callContext, args map[string]json.RawMessage) string {
	switch {
	case call.Command != "":
		return truncatePreview("command: " + call.Command)
	case call.Path != "":
		return truncatePreview("path: " + call.Path)
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return truncatePreview("parameters: " + strings.Join(keys, ", "))
}

func truncatePreview(value string) string {
	const limit = 240
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func (p *plugin) listPending(_ context.Context, req *pluginsdk.Request) (pendingListOut, error) {
	var in pendingListIn
	if len(req.Payload) > 0 {
		if err := DecodeStrict(req.Payload, &in); err != nil {
			return pendingListOut{}, err
		}
	}
	return pendingListOut{Pending: p.pending.list(in.SessionID, time.Now())}, nil
}

func (p *plugin) decideCall(_ context.Context, req *pluginsdk.Request) (decideOut, error) {
	var in decideIn
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return decideOut{}, err
	}
	if in.Token == "" {
		return decideOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "token is required")
	}
	if in.Decision != ActionAllow && in.Decision != ActionDeny {
		return decideOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
			"decision must be allow or deny")
	}
	item, err := p.pending.decide(in.Token, in.Decision, time.Now())
	if err != nil {
		p.publishPending()
		if item != nil {
			return decideOut{Token: item.Token, Decision: in.Decision, State: "expired",
				ExpiresAt: item.ExpiresTS}, err
		}
		return decideOut{}, err
	}
	p.publishPending()
	state := "rejected"
	if in.Decision == ActionAllow {
		state = "approved"
	}
	return decideOut{
		Token: item.Token, Decision: in.Decision, State: state, ExpiresAt: item.ExpiresTS,
	}, nil
}

// publishPending pushes the complete pending list to the confirm mount. The
// component discards older revisions, so a burst of decisions cannot reorder
// the panel.
func (p *plugin) publishPending() {
	p.mu.Lock()
	ui := p.ui
	p.uiRev++
	revision := p.uiRev
	cfg := p.cfg
	p.mu.Unlock()
	if ui == nil {
		return
	}
	views := p.pending.list("", time.Now())
	state := confirmStateUI{
		Revision: int(revision),
		Pending:  views,
		Policy: confirmPolicyUI{
			DefaultAction: cfg.DefaultAction,
			RuleCount:     len(p.rules.list("all", "")),
			PendingCount:  len(views),
		},
	}
	body, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = ui.SetState("confirm", body)
}

func confirmExpiredResult(verdict decision, expiresAt int64) (json.RawMessage, error) {
	content := fmt.Sprintf(
		"Tool call was not run: the user did not confirm it before the token expired.\nTool: %s\nReason: %s",
		verdict.Tool, verdict.Reason)
	meta := map[string]any{
		"code":              CodeConfirmExpired,
		"sandbox_decision":  ActionConfirm,
		"rule_id":           verdict.RuleID,
		"reason":            "confirmation expired",
		"risk":              verdict.Risk,
		"tool":              verdict.Tool,
		"target_capability": verdict.Capability,
		"target_method":     verdict.Method,
		"expires_at":        expiresAt,
	}
	return encodeResult(toolResult{Content: content, IsError: true, Meta: meta})
}
