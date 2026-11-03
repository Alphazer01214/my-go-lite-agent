package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type seamProceed func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)

func (p *plugin) handleSeam(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var payload seamPayload
	if err := DecodeSeam(req.Payload, &payload); err != nil {
		return nil, err
	}
	return p.adjudicate(ctx, req, payload, func(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
		if err := req.Proceed(ctx, body); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

func (p *plugin) adjudicate(
	ctx context.Context,
	req *pluginsdk.Request,
	payload seamPayload,
	proceed seamProceed,
) (json.RawMessage, error) {
	isToolCall := req.Capability == "tool" && req.Method == "call"
	call, err := p.resolveCall(ctx, req, payload)
	if err != nil {
		return nil, err
	}

	// tools dispatches a tool by calling its target capability.method with the
	// same call_id. The outer tool.call already adjudicated this call, so the
	// nested provider seam must not ask again or write a duplicate audit.
	if !isToolCall && p.approved.consume(call, time.Now()) {
		return proceed(ctx, req.Payload)
	}

	verdict := p.decide(call)
	if verdict.Action == ActionConfirm {
		return p.awaitConfirmation(ctx, req, payload, verdict, call, proceed)
	}
	p.writeAudit(ctx, verdict, call, payload.Arguments, "", 0)

	switch verdict.Action {
	case ActionAllow:
		p.rememberApproved(isToolCall, call)
		return proceed(ctx, req.Payload)
	case ActionModify:
		arguments, err := applyTemplate(payload.Arguments, verdict.ModifiedArguments)
		if err != nil {
			return nil, err
		}
		rewritten, err := withArguments(req.Payload, arguments)
		if err != nil {
			return nil, err
		}
		p.rememberApproved(isToolCall, call)
		return proceed(ctx, rewritten)
	default:
		return denyResult(verdict)
	}
}

func (p *plugin) resolveCall(
	ctx context.Context,
	req *pluginsdk.Request,
	payload seamPayload,
) (callContext, error) {
	call := callContext{
		SessionID: payload.SessionID,
		Workspace: payload.Workspace,
		CallID:    payload.CallID,
	}
	index := p.toolIndex(ctx)

	if req.Capability == "tool" && req.Method == "call" {
		call.Tool = payload.Name
		if entry, ok := index.lookup(payload.Name); ok {
			call.Capability = entry.Capability
			call.Method = entry.Method
		}
	} else {
		call.Capability = req.Capability
		call.Method = req.Method
		if entry, ok := index.lookupTarget(req.Capability, req.Method); ok {
			call.Tool = entry.Name
		}
	}

	args, err := Arguments(payload.Arguments)
	if err != nil {
		return callContext{}, err
	}
	switch call.Capability {
	case "file":
		call.Path = ArgumentString(args, "path")
	case "shell":
		call.Command = commandLine(args)
	case "python":
		call.Path = ArgumentString(args, "path")
		if call.Path == "" {
			call.Command = strings.TrimSpace(ArgumentString(args, "code"))
		}
	}
	return call, nil
}

func (p *plugin) rememberApproved(isToolCall bool, call callContext) {
	if !isToolCall {
		return
	}
	p.approved.remember(call, time.Now())
}

func denyResult(verdict decision) (json.RawMessage, error) {
	content := "Tool call denied by policy."
	if verdict.Tool != "" {
		content += "\nTool: " + verdict.Tool
	}
	content += "\nReason: " + verdict.Reason
	if verdict.RuleID != "" {
		content += "\nRule: " + verdict.RuleID
	}
	return encodeResult(toolResult{
		Content: content,
		IsError: true,
		Meta: map[string]any{
			"code":              CodeDenied,
			"sandbox_decision":  verdict.Action,
			"rule_id":           verdict.RuleID,
			"reason":            verdict.Reason,
			"tool":              verdict.Tool,
			"target_capability": verdict.Capability,
			"target_method":     verdict.Method,
		},
	})
}

func confirmExpiredResult(verdict decision, expiresAt int64) (json.RawMessage, error) {
	content := fmt.Sprintf(
		"Tool call was not run: the user did not confirm it before the token expired.\nTool: %s\nReason: %s",
		verdict.Tool, verdict.Reason)
	return encodeResult(toolResult{
		Content: content,
		IsError: true,
		Meta: map[string]any{
			"code":              CodeConfirmExpired,
			"sandbox_decision":  ActionConfirm,
			"rule_id":           verdict.RuleID,
			"reason":            "confirmation expired",
			"tool":              verdict.Tool,
			"target_capability": verdict.Capability,
			"target_method":     verdict.Method,
			"expires_at":        expiresAt,
		},
	})
}
