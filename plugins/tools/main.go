package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

func handleTyped[T any](fn func(context.Context, *pluginsdk.Request) (T, error)) pluginsdk.HandleFunc {
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

	sdk.Register("tools", "sync", pluginsdk.NewHandler(handleTyped(p.sync)).
		WithInfo("sync", "atomically replace the caller's complete tool set"))
	sdk.Register("tools", "register", pluginsdk.NewHandler(handleTyped(p.register)).
		WithInfo("register", "register or replace one tool"))
	sdk.Register("tools", "unregister", pluginsdk.NewHandler(handleTyped(p.unregister)).
		WithInfo("unregister", "remove one tool owned by the caller"))
	sdk.Register("tools", "list", pluginsdk.NewHandler(handleTyped(p.list)).
		WithInfo("list", "list currently available tools"))
	sdk.Register("tools", "call", pluginsdk.NewHandler(p.call).
		WithInfo("call", "dispatch one model tool call"))

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type toolsPlugin struct {
	sdk          sdkCaller
	reg          *registry
	registryPath string
}

func newToolsPlugin(sdk sdkCaller) *toolsPlugin {
	path := os.Getenv("TOOLS_REGISTRY_FILE")
	if path == "" {
		path = registryFile
	}
	reg, err := loadRegistry(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools: load registry: %v\n", err)
	}
	return &toolsPlugin{sdk: sdk, reg: reg, registryPath: path}
}

func (p *toolsPlugin) persist() {
	if p.registryPath == "" {
		return
	}
	if err := p.reg.save(p.registryPath); err != nil {
		fmt.Fprintf(os.Stderr, "tools: persist registry: %v\n", err)
	}
}

func (p *toolsPlugin) sync(ctx context.Context, req *pluginsdk.Request) (SyncOutput, error) {
	source := req.From
	if source == "" {
		return SyncOutput{}, pluginsdk.ErrCode("tool_source_invalid", "request source is required")
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
			return SyncOutput{}, pluginsdk.ErrCode("tool_source_invalid",
				fmt.Sprintf("%s does not provide %s", source, definition.TargetCapability))
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
		return RegisterOutput{}, pluginsdk.ErrCode("tool_source_invalid", "request source is required")
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
		return RegisterOutput{}, pluginsdk.ErrCode("tool_source_invalid",
			fmt.Sprintf("%s does not provide %s", source, definition.TargetCapability))
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
		return out, pluginsdk.ErrCode("tool_source_invalid", "request source is required")
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
		return out, pluginsdk.ErrCode("tool_target_unavailable", "cannot verify tool owners: "+err.Error())
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
		return nil, pluginsdk.ErrCode("tool_not_found", "tool not found: "+in.Name)
	}
	definition := entry.definition
	snapshot, err := p.hostPlugins(ctx)
	if err != nil {
		return nil, pluginsdk.ErrCode("tool_target_unavailable", "cannot verify tool owner: "+err.Error())
	}
	if ok, reason := snapshot.availability(
		definition.SourcePlugin, definition.TargetCapability, definition.TargetMethod, entry.generation,
	); !ok {
		return nil, pluginsdk.ErrCode("tool_target_unavailable", reason)
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
