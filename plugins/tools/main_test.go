package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type fakeToolsSDK struct {
	mu          sync.Mutex
	snapshot    hostSnapshot
	target      json.RawMessage
	targetCap   string
	targetMeth  string
	contextID   string
	callPayload json.RawMessage
}

func (f *fakeToolsSDK) Call(_ context.Context, capability, method string, _ json.RawMessage) (json.RawMessage, error) {
	if capability == "host" && method == "plugins" {
		return json.Marshal(f.snapshot)
	}
	return nil, pluginsdk.ErrCode(pluginsdk.CodeMethodNotFound, "unexpected call")
}

func (f *fakeToolsSDK) CallInContext(_ context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contextID = contextID
	f.targetCap = capability
	f.targetMeth = method
	f.callPayload = append(json.RawMessage(nil), payload...)
	return f.target, nil
}

func activeSnapshot(name, capability, method string) hostSnapshot {
	return hostSnapshot{Plugins: []pluginSnapshot{{
		Name: name, State: "active", Enabled: true, Provides: []string{capability},
		Instances: []instanceSnapshot{{
			State:         "active",
			Registrations: []registrationSnapshot{{Capability: capability, Method: method}},
		}},
	}}}
}

func testDefinition(name, targetCapability, targetMethod, risk string) toolcontract.RegisterInput {
	return toolcontract.RegisterInput{
		Name:             name,
		Description:      "test tool",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		TargetCapability: targetCapability,
		TargetMethod:     targetMethod,
		Risk:             risk,
	}
}

func TestRegisterStoresTrustedSourceAndUpdates(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read")}
	p := newToolsPlugin(sdk)
	body, _ := json.Marshal(testDefinition("file_read", "file", "read", "low"))
	req := &pluginsdk.Request{From: "file-tools", Payload: body}

	out, err := p.register(context.Background(), req)
	if err != nil || !out.Registered || out.Updated {
		t.Fatalf("register = %+v, %v", out, err)
	}
	def, ok := p.reg.get("file_read")
	if !ok || def.SourcePlugin != "file-tools" {
		t.Fatalf("definition = %+v", def)
	}
	out, err = p.register(context.Background(), req)
	if err != nil || !out.Updated {
		t.Fatalf("update = %+v, %v", out, err)
	}
}

func TestRegisterRejectsConflictAndReservedTarget(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: hostSnapshot{Plugins: []pluginSnapshot{
		{Name: "file-tools", State: "active", Enabled: true, Provides: []string{"file"},
			Instances: []instanceSnapshot{{State: "active", Registrations: []registrationSnapshot{{Capability: "file", Method: "read"}}}}},
		{Name: "shell-tools", State: "active", Enabled: true, Provides: []string{"shell"},
			Instances: []instanceSnapshot{{State: "active", Registrations: []registrationSnapshot{{Capability: "shell", Method: "execute"}}}}},
	}}}
	p := newToolsPlugin(sdk)
	first, _ := json.Marshal(testDefinition("shared", "file", "read", "low"))
	if _, err := p.register(context.Background(), &pluginsdk.Request{From: "file-tools", Payload: first}); err != nil {
		t.Fatal(err)
	}
	second, _ := json.Marshal(testDefinition("shared", "shell", "execute", "high"))
	_, err := p.register(context.Background(), &pluginsdk.Request{From: "shell-tools", Payload: second})
	if pluginsdk.Code(err) != "tool_name_conflict" {
		t.Fatalf("error = %v", err)
	}

	reserved, _ := json.Marshal(testDefinition("bad_target", "tools", "call", "low"))
	_, err = p.register(context.Background(), &pluginsdk.Request{From: "file-tools", Payload: reserved})
	if pluginsdk.Code(err) != "tool_target_reserved" {
		t.Fatalf("reserved error = %v", err)
	}
}

func TestListHidesUnavailableProviders(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read")}
	p := newToolsPlugin(sdk)
	p.reg.put(toolcontract.Definition{
		Name: "file_read", Description: "read", Risk: "low", SourcePlugin: "file-tools",
		TargetCapability: "file", TargetMethod: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	})
	p.reg.put(toolcontract.Definition{
		Name: "shell_execute", Description: "shell", Risk: "high", SourcePlugin: "shell-tools",
		TargetCapability: "shell", TargetMethod: "execute", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	})

	out, err := p.list(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "file_read" {
		t.Fatalf("tools = %+v", out.Tools)
	}
}

func TestCallPreservesContextAndValidatesResult(t *testing.T) {
	result, _ := toolcontract.Success("hello", map[string]any{"path": "README.md"})
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read"), target: result}
	p := newToolsPlugin(sdk)
	p.reg.put(toolcontract.Definition{
		Name: "file_read", Description: "read", Risk: "low", SourcePlugin: "file-tools",
		TargetCapability: "file", TargetMethod: "read", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	})
	body := json.RawMessage(`{"name":"file_read","arguments":{"path":"README.md"},"workspace":"E:/project","call_id":"c1"}`)
	raw, err := p.call(context.Background(), &pluginsdk.Request{ContextID: "ctx-1", Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := toolcontract.ParseResult(raw)
	if err != nil || parsed.Content != "hello" {
		t.Fatalf("result = %+v, %v", parsed, err)
	}
	if sdk.contextID != "ctx-1" || sdk.targetCap != "file" || sdk.targetMeth != "read" {
		t.Fatalf("target call = ctx=%s %s.%s", sdk.contextID, sdk.targetCap, sdk.targetMeth)
	}
	var provider toolcontract.ProviderInput
	if err := json.Unmarshal(sdk.callPayload, &provider); err != nil {
		t.Fatal(err)
	}
	if provider.Workspace != "E:/project" || provider.CallID != "c1" {
		t.Fatalf("provider input = %+v", provider)
	}
}
