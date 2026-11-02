package main

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	codeToolNotFound      = "tool_not_found"
	codeToolNameConflict  = "tool_name_conflict"
	codeToolResultInvalid = "tool_result_invalid"
)

func (p *plugin) list(ctx context.Context) (*listResponse, error) {
	current, err := p.ensureCatalog(ctx, false)
	if err != nil {
		return nil, ctxError(err)
	}
	response := current.response()
	return &response, nil
}

func (p *plugin) call(ctx context.Context, req *pluginsdk.Request, in *callRequest) (json.RawMessage, error) {
	if !toolNamePattern.MatchString(in.Name) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "invalid tool name")
	}
	if len(in.Arguments) > maxArgumentsBytes {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "arguments exceed size limit")
	}
	arguments, err := normalizeObject(in.Arguments, "arguments")
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}

	var meta json.RawMessage
	if trimmed := bytes.TrimSpace(in.Meta); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if len(trimmed) > maxMetaBytes {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "meta exceeds size limit")
		}
		meta, err = normalizeObject(in.Meta, "meta")
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
	}

	current, err := p.ensureCatalog(ctx, false)
	if err != nil {
		return nil, ctxError(err)
	}
	tool, ok := current.ByName[in.Name]
	if !ok {
		current, err = p.ensureCatalog(ctx, true)
		if err != nil {
			return nil, ctxError(err)
		}
		if tool, ok = current.ByName[in.Name]; !ok {
			return nil, pluginsdk.ErrCode(codeToolNotFound, "tool not found: "+in.Name)
		}
	}

	payload, err := json.Marshal(providerCall{
		Arguments: arguments,
		SessionID: in.SessionID,
		Workspace: in.Workspace,
		CallID:    in.CallID,
		Meta:      meta,
	})
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}

	var raw json.RawMessage
	if req.ContextID != "" {
		raw, err = p.sdk.CallInContext(ctx, req.ContextID, tool.Capability, tool.Method, payload)
	} else {
		raw, err = p.sdk.Call(ctx, tool.Capability, tool.Method, payload)
	}
	if err != nil {
		return nil, err
	}
	if err := validateToolResult(raw); err != nil {
		return nil, pluginsdk.ErrCode(codeToolResultInvalid, err.Error())
	}
	return raw, nil
}
