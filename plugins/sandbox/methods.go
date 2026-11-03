package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleCheck(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input checkIn
	if err := DecodeStrict(req.Payload, &input); err != nil {
		return nil, err
	}
	out, err := p.check(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (p *plugin) check(ctx context.Context, in *checkIn) (checkOut, error) {
	if err := ctx.Err(); err != nil {
		return checkOut{}, err
	}
	if in.Tool == "" && (in.Capability == "" || in.Method == "") {
		return checkOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
			"tool or capability and method are required")
	}

	index := p.toolIndex(ctx)
	call := callContext{SessionID: in.SessionID, Workspace: in.Workspace}
	if in.Tool != "" {
		call.Tool = in.Tool
		if entry, ok := index.lookup(in.Tool); ok {
			if in.Capability != "" && in.Capability != entry.Capability {
				return checkOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
					"tool and capability name different targets")
			}
			if in.Method != "" && in.Method != entry.Method {
				return checkOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
					"tool and method name different targets")
			}
			call.Capability = entry.Capability
			call.Method = entry.Method
		} else {
			call.Capability = in.Capability
			call.Method = in.Method
		}
	} else {
		call.Capability = in.Capability
		call.Method = in.Method
		if entry, ok := index.lookupTarget(in.Capability, in.Method); ok {
			call.Tool = entry.Name
		}
	}

	args, err := Arguments(in.Arguments)
	if err != nil {
		return checkOut{}, err
	}
	switch call.Capability {
	case "file":
		call.Path = ArgumentString(args, "path")
	case "shell":
		call.Command = commandLine(args)
	case "python":
		call.Path = ArgumentString(args, "path")
		if call.Path == "" {
			call.Command = ArgumentString(args, "code")
		}
	}

	verdict := p.decide(call)
	out := checkOut{
		Decision: verdict.Action,
		RuleID:   verdict.RuleID,
		Reason:   verdict.Reason,
	}
	if verdict.Action == ActionModify {
		out.ModifiedArguments = verdict.ModifiedArguments
	}
	return out, nil
}

func (p *plugin) handlePendingList(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	out, err := p.listPending(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (p *plugin) handleDecide(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	out, err := p.decideCall(ctx, req)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
