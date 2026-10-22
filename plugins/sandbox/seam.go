package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"strings"
	"time"
)

func (p *plugin) handleSeam(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var payload seamPayload
	if err := DecodeSeam(req.Payload, &payload); err != nil {
		return nil, err
	}
	return p.adjudicate(ctx, req, payload, func(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
		return req.Proceed(ctx, body)
	})
}

type seamProceed func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)

// adjudicate is the seam handler without the SDK plumbing.
func (p *plugin) adjudicate(
	ctx context.Context,
	req *pluginsdk.Request,
	payload seamPayload,
	proceed seamProceed,
) (json.RawMessage, error) {
	call, risk, err := p.resolveCall(ctx, req, payload)
	if err != nil {
		return nil, err
	}
	// tools dispatching a tool forwards the same call_id into the direct
	// provider seam. The outer tools.call already adjudicated this call, so do
	// not ask a second time (or write a duplicate audit entry).
	if req.Capability == "tools" && req.Method == "call" {
		// This is the outer call; it owns its one adjudication below.
	} else if p.approved.consume(call, time.Now()) {
		return proceed(ctx, req.Payload)
	}

	verdict := p.decide(call, risk)
	if verdict.Action == ActionConfirm {
		return p.awaitConfirmation(ctx, req, payload, verdict, call, proceed)
	}
	p.writeAudit(ctx, verdict, call, payload.Arguments, "", 0)

	switch verdict.Action {
	case ActionAllow:
		p.rememberApproved(req, call)
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
		p.rememberApproved(req, call)
		return proceed(ctx, rewritten)
	default:
		return denyResult(verdict)
	}
}

// rememberApproved marks an allowed tools.call so its nested direct provider
// seam skips adjudication.
func (p *plugin) rememberApproved(req *pluginsdk.Request, call callContext) {
	if req.Capability != "tools" || req.Method != "call" {
		return
	}
	p.approved.remember(call, time.Now())
}

func (p *plugin) resolveCall(ctx context.Context, req *pluginsdk.Request, payload seamPayload) (callContext, int, error) {
	table := p.toolTable(ctx)
	call := callContext{
		SessionID: payload.SessionID,
		Workspace: payload.Workspace,
		CallID:    payload.CallID,
	}
	risk := UnknownRisk

	if req.Capability == "tools" && req.Method == "call" {
		call.Tool = payload.Name
		if entry, ok := table.lookup(payload.Name); ok {
			call.Capability = entry.TargetCapability
			call.Method = entry.TargetMethod
			risk = entry.Risk
		}
	} else {
		call.Capability = req.Capability
		call.Method = req.Method
		if entry, ok := table.lookupTarget(req.Capability, req.Method); ok {
			call.Tool = entry.Name
			risk = entry.Risk
		}
	}

	args, err := Arguments(payload.Arguments)
	if err != nil {
		return callContext{}, 0, err
	}
	switch call.Capability {
	case "file":
		call.Path = ArgumentString(args, "path")
	case "shell":
		call.Command = commandLine(args)
	}
	return call, risk, nil
}

func (p *plugin) decide(call callContext, risk int) decision {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()
	now := time.Now()
	return evaluate(p.rules.activeRules(call.SessionID, now), cfg, call, risk, now)
}

func commandLine(args map[string]json.RawMessage) string {
	command := ArgumentString(args, "command")
	extra := ArgumentStrings(args, "args")
	if len(extra) == 0 {
		return command
	}
	return strings.Join(append([]string{command}, extra...), " ")
}

func denyResult(verdict decision) (json.RawMessage, error) {
	content := fmt.Sprintf("Tool call denied by policy.\nTool: %s\nRisk: %d\nReason: %s",
		verdict.Tool, verdict.Risk, verdict.Reason)
	if verdict.RuleID != "" {
		content += "\nRule: " + verdict.RuleID
	}
	meta := map[string]any{
		"code":              CodeDenied,
		"sandbox_decision":  verdict.Action,
		"rule_id":           verdict.RuleID,
		"reason":            verdict.Reason,
		"risk":              verdict.Risk,
		"tool":              verdict.Tool,
		"target_capability": verdict.Capability,
		"target_method":     verdict.Method,
	}
	return encodeResult(toolResult{Content: content, IsError: true, Meta: meta})
}
