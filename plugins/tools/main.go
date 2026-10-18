package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

func handle(fn func(context.Context, *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(ctx, req)
		if err != nil {
			return nil, err
		}
		body, err := json.Marshal(out)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
		}
		return body, nil
	}
}

func main() {
	sdk := pluginsdk.NewPlugin("tools")
	p := newToolsPlugin(sdk)

	sdk.Register("tools", "register", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		return p.register(ctx, req)
	})).WithInfo("register", "register or replace a Tool Definition"))
	sdk.Register("tools", "unregister", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		return p.unregister(req)
	})).WithInfo("unregister", "remove a Tool Definition owned by the caller"))
	sdk.Register("tools", "list", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		return p.list(ctx)
	})).WithInfo("list", "list available Tool Definitions"))
	sdk.Register("tools", "call", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		return p.call(ctx, req)
	})).WithInfo("call", "dispatch one model Tool Call"))

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type toolsPlugin struct {
	sdk sdkCaller
	reg *registry
}

func newToolsPlugin(sdk sdkCaller) *toolsPlugin {
	return &toolsPlugin{sdk: sdk, reg: newRegistry()}
}

type registerOut struct {
	Registered bool `json:"registered"`
	Updated    bool `json:"updated"`
}

func (p *toolsPlugin) register(ctx context.Context, req *pluginsdk.Request) (registerOut, error) {
	if req.From == "" {
		return registerOut{}, pluginsdk.ErrCode("tool_source_invalid", "request source is required")
	}
	var in toolcontract.RegisterInput
	if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
		return registerOut{}, err
	}
	def, err := validateDefinition(in)
	if err != nil {
		return registerOut{}, err
	}
	def.SourcePlugin = req.From
	if err := p.requireSource(ctx, def.SourcePlugin, def.TargetCapability, def.TargetMethod); err != nil {
		return registerOut{}, err
	}
	updated, err := p.reg.put(def)
	if err != nil {
		return registerOut{}, err
	}
	return registerOut{Registered: true, Updated: updated}, nil
}

type unregisterIn struct {
	Name string `json:"name"`
}

type unregisterOut struct {
	Unregistered bool `json:"unregistered"`
}

func (p *toolsPlugin) unregister(req *pluginsdk.Request) (unregisterOut, error) {
	if req.From == "" {
		return unregisterOut{}, pluginsdk.ErrCode("tool_source_invalid", "request source is required")
	}
	var in unregisterIn
	if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
		return unregisterOut{}, err
	}
	if !validToolName(in.Name) {
		return unregisterOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "invalid tool name")
	}
	found, err := p.reg.remove(in.Name, req.From)
	if err != nil {
		return unregisterOut{}, err
	}
	return unregisterOut{Unregistered: found}, nil
}

type listOut struct {
	Tools []toolcontract.Definition `json:"tools"`
}

func (p *toolsPlugin) list(ctx context.Context) (listOut, error) {
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return listOut{Tools: p.reg.snapshot()}, nil
	}
	return listOut{Tools: p.reg.available(snapshot)}, nil
}

func (p *toolsPlugin) call(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var in toolcontract.CallInput
	if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
		return nil, err
	}
	if !validToolName(in.Name) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "invalid tool name")
	}
	arguments, err := toolcontract.Object(in.Arguments, "arguments")
	if err != nil {
		return nil, err
	}
	def, ok := p.reg.get(in.Name)
	if !ok {
		return nil, pluginsdk.ErrCode("tool_not_found", "tool not found: "+in.Name)
	}
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return nil, pluginsdk.ErrCode("tool_target_unavailable", "cannot verify tool owner: "+err.Error())
	}
	if !snapshot.available(def.SourcePlugin, def.TargetCapability, def.TargetMethod) {
		return nil, pluginsdk.ErrCode("tool_target_unavailable", "tool owner is unavailable: "+def.SourcePlugin)
	}

	payload, err := json.Marshal(toolcontract.ProviderInput{
		Arguments: arguments,
		Workspace: in.Workspace,
		CallID:    in.CallID,
	})
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	var raw json.RawMessage
	if req.ContextID != "" {
		raw, err = p.sdk.CallInContext(ctx, req.ContextID, def.TargetCapability, def.TargetMethod, payload)
	} else {
		raw, err = p.sdk.Call(ctx, def.TargetCapability, def.TargetMethod, payload)
	}
	if err != nil {
		return nil, err
	}
	if _, err := toolcontract.ParseResult(raw); err != nil {
		return nil, pluginsdk.ErrCode("tool_result_invalid", err.Error())
	}
	return raw, nil
}
