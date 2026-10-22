package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *toolsPlugin) sync(ctx context.Context, req *pluginsdk.Request) (SyncOutput, error) {
	source := req.From
	if source == "" {
		return SyncOutput{}, errToolSourceInvalid("request source is required")
	}
	var in SyncInput
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return SyncOutput{}, err
	}
	snapshot, generation, err := p.sourceGeneration(ctx, source)
	if err != nil {
		return SyncOutput{}, err
	}
	plugin, _ := snapshot.plugin(source)

	definitions := make([]Definition, 0, len(in.Tools))
	for _, item := range in.Tools {
		definition, err := validateDefinition(item)
		if err != nil {
			return SyncOutput{}, err
		}
		if !containsString(plugin.Provides, definition.TargetCapability) {
			return SyncOutput{}, errToolSourceInvalid(fmt.Sprintf("%s does not provide %s", source, definition.TargetCapability))
		}
		definition.SourcePlugin = source
		definitions = append(definitions, definition)
	}

	added, updated, removed, err := p.reg.replaceSource(source, definitions, generation)
	if err != nil {
		return SyncOutput{}, err
	}
	p.persist()
	return SyncOutput{Synced: true, Added: added, Updated: updated, Removed: removed}, nil
}

func (p *toolsPlugin) register(ctx context.Context, req *pluginsdk.Request) (RegisterOutput, error) {
	source := req.From
	if source == "" {
		return RegisterOutput{}, errToolSourceInvalid("request source is required")
	}
	var in RegisterInput
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return RegisterOutput{}, err
	}
	snapshot, generation, err := p.sourceGeneration(ctx, source)
	if err != nil {
		return RegisterOutput{}, err
	}
	plugin, _ := snapshot.plugin(source)
	definition, err := validateDefinition(in)
	if err != nil {
		return RegisterOutput{}, err
	}
	if !containsString(plugin.Provides, definition.TargetCapability) {
		return RegisterOutput{}, errToolSourceInvalid(fmt.Sprintf("%s does not provide %s", source, definition.TargetCapability))
	}
	definition.SourcePlugin = source
	updated, err := p.reg.put(definition, generation)
	if err != nil {
		return RegisterOutput{}, err
	}
	p.persist()
	return RegisterOutput{Registered: true, Updated: updated}, nil
}

func (p *toolsPlugin) unregister(_ context.Context, req *pluginsdk.Request) (struct {
	Unregistered bool `json:"unregistered"`
}, error) {
	var out struct {
		Unregistered bool `json:"unregistered"`
	}
	if req.From == "" {
		return out, errToolSourceInvalid("request source is required")
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return out, err
	}
	if !validToolName(in.Name) {
		return out, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "invalid tool name")
	}
	found, err := p.reg.remove(in.Name, req.From)
	if err != nil {
		return out, err
	}
	if found {
		p.persist()
	}
	out.Unregistered = found
	return out, nil
}

func (p *toolsPlugin) list(ctx context.Context, _ *pluginsdk.Request) (struct {
	Tools []Definition `json:"tools"`
}, error) {
	var out struct {
		Tools []Definition `json:"tools"`
	}
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return out, errToolTargetUnavailable("cannot verify tool owners: " + err.Error())
	}
	out.Tools = p.reg.available(snapshot)
	if out.Tools == nil {
		out.Tools = []Definition{}
	}
	return out, nil
}

func (p *toolsPlugin) call(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var in CallInput
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return nil, err
	}
	if !validToolName(in.Name) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "invalid tool name")
	}
	arguments, err := Object(in.Arguments, "arguments")
	if err != nil {
		return nil, err
	}
	entry, ok := p.reg.get(in.Name)
	if !ok {
		return nil, errToolNotFound("tool not found: " + in.Name)
	}
	definition := entry.definition
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return nil, errToolTargetUnavailable("cannot verify tool owner: " + err.Error())
	}
	if ok, reason := snapshot.availability(
		definition.SourcePlugin, definition.TargetCapability, definition.TargetMethod, entry.generation,
	); !ok {
		return nil, errToolTargetUnavailable(reason)
	}

	payload, err := json.Marshal(ProviderInput{
		Arguments:   arguments,
		SessionID:   in.SessionID,
		Workspace:   in.Workspace,
		WorkspaceID: in.WorkspaceID,
		CallID:      in.CallID,
		Mode:        in.Mode,
	})
	if err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	var raw json.RawMessage
	if req.ContextID != "" {
		raw, err = p.sdk.CallInContext(ctx, req.ContextID, definition.TargetCapability, definition.TargetMethod, payload)
	} else {
		raw, err = p.sdk.Call(ctx, definition.TargetCapability, definition.TargetMethod, payload)
	}
	if err != nil {
		return nil, err
	}
	if _, err := ParseResult(raw); err != nil {
		return nil, pluginsdk.ErrCode("tool_result_invalid", err.Error())
	}
	return raw, nil
}
