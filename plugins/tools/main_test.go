package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
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

// newTestToolsPlugin disables the on-disk registry cache so tests stay isolated.
func newTestToolsPlugin(sdk sdkCaller) *toolsPlugin {
	return &toolsPlugin{sdk: sdk, reg: newRegistry()}
}

func activeSnapshot(name, capability, method string, generation int) hostSnapshot {
	return hostSnapshot{Plugins: []pluginSnapshot{{
		Name: name, State: "active", Enabled: true, Provides: []string{capability},
		Instances: []instanceSnapshot{{
			State:         "active",
			Generation:    generation,
			Registrations: []registrationSnapshot{{Capability: capability, Method: method}},
		}},
	}}}
}

func testDefinition(name, targetCapability, targetMethod string, risk int) RegisterInput {
	return RegisterInput{
		Name:             name,
		Description:      "test tool",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		TargetCapability: targetCapability,
		TargetMethod:     targetMethod,
		Risk:             risk,
		Tags:             []string{"test"},
	}
}

func syncPayload(t *testing.T, definitions ...RegisterInput) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(SyncInput{Tools: definitions})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSyncReplacesSourceSet(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1)}
	p := newTestToolsPlugin(sdk)
	out, err := p.sync(context.Background(), &pluginsdk.Request{
		From: "file-tools",
		Payload: syncPayload(t,
			testDefinition("file_read", "file", "read", RiskLow),
			testDefinition("file_write", "file", "read", RiskHigh),
		),
	})
	if err != nil || !out.Synced || out.Added != 2 {
		t.Fatalf("sync = %+v, %v", out, err)
	}
	entry, ok := p.reg.get("file_read")
	if !ok || entry.definition.SourcePlugin != "file-tools" || entry.generation != 1 {
		t.Fatalf("entry = %+v", entry)
	}

	out, err = p.sync(context.Background(), &pluginsdk.Request{
		From: "file-tools", Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	})
	if err != nil || out.Removed != 1 {
		t.Fatalf("resync = %+v, %v", out, err)
	}
	if _, ok := p.reg.get("file_write"); ok {
		t.Fatal("file_write survived a full sync")
	}
}

func TestSyncRejectsForeignTargetAtomically(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1)}
	p := newTestToolsPlugin(sdk)
	_, err := p.sync(context.Background(), &pluginsdk.Request{
		From: "file-tools",
		Payload: syncPayload(t,
			testDefinition("file_read", "file", "read", RiskLow),
			testDefinition("bad", "shell", "execute", RiskHigh),
		),
	})
	if pluginsdk.Code(err) != "tool_source_invalid" {
		t.Fatalf("err = %v", err)
	}
	if _, ok := p.reg.get("file_read"); ok {
		t.Fatal("partial sync was applied")
	}
}

func TestRegisterStoresTrustedSourceAndGeneration(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 3)}
	p := newTestToolsPlugin(sdk)
	body, _ := json.Marshal(testDefinition("file_read", "file", "read", RiskLow))
	out, err := p.register(context.Background(), &pluginsdk.Request{From: "file-tools", Payload: body})
	if err != nil || !out.Registered || out.Updated {
		t.Fatalf("register = %+v, %v", out, err)
	}
	entry, ok := p.reg.get("file_read")
	if !ok || entry.definition.SourcePlugin != "file-tools" || entry.generation != 3 {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestRegisterRejectsConflictAndReservedTarget(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: hostSnapshot{Plugins: []pluginSnapshot{
		{Name: "file-tools", State: "active", Enabled: true, Provides: []string{"file"},
			Instances: []instanceSnapshot{{State: "active", Generation: 1, Registrations: []registrationSnapshot{{Capability: "file", Method: "read"}}}}},
		{Name: "shell-tools", State: "active", Enabled: true, Provides: []string{"shell"},
			Instances: []instanceSnapshot{{State: "active", Generation: 1, Registrations: []registrationSnapshot{{Capability: "shell", Method: "execute"}}}}},
	}}}
	p := newTestToolsPlugin(sdk)
	first, _ := json.Marshal(testDefinition("shared", "file", "read", RiskLow))
	if _, err := p.register(context.Background(), &pluginsdk.Request{From: "file-tools", Payload: first}); err != nil {
		t.Fatal(err)
	}
	second, _ := json.Marshal(testDefinition("shared", "shell", "execute", RiskHigh))
	_, err := p.register(context.Background(), &pluginsdk.Request{From: "shell-tools", Payload: second})
	if pluginsdk.Code(err) != "tool_name_conflict" {
		t.Fatalf("error = %v", err)
	}

	reserved, _ := json.Marshal(testDefinition("bad_target", "tools", "call", RiskLow))
	_, err = p.register(context.Background(), &pluginsdk.Request{From: "file-tools", Payload: reserved})
	if pluginsdk.Code(err) != "tool_target_reserved" {
		t.Fatalf("reserved error = %v", err)
	}
}

func TestListHidesStaleGeneration(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1)}
	p := newTestToolsPlugin(sdk)
	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From:    "file-tools",
		Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	}); err != nil {
		t.Fatal(err)
	}
	out, err := p.list(context.Background(), nil)
	if err != nil || len(out.Tools) != 1 {
		t.Fatalf("list = %+v, %v", out, err)
	}

	sdk.snapshot = activeSnapshot("file-tools", "file", "read", 2)
	out, err = p.list(context.Background(), nil)
	if err != nil || len(out.Tools) != 0 {
		t.Fatalf("stale list = %+v, %v", out, err)
	}

	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From:    "file-tools",
		Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	}); err != nil {
		t.Fatal(err)
	}
	out, err = p.list(context.Background(), nil)
	if err != nil || len(out.Tools) != 1 {
		t.Fatalf("resynced list = %+v, %v", out, err)
	}
}

func TestListHidesUnavailableProviders(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1)}
	p := newTestToolsPlugin(sdk)
	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From:    "file-tools",
		Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	}); err != nil {
		t.Fatal(err)
	}
	sdk.snapshot = activeSnapshot("other-tools", "file", "read", 1)
	out, err := p.list(context.Background(), nil)
	if err != nil || len(out.Tools) != 0 {
		t.Fatalf("tools = %+v, %v", out, err)
	}
}

func TestCallPreservesContextAndPayload(t *testing.T) {
	result, _ := Success("hello", map[string]any{"path": "README.md"})
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1), target: result}
	p := newTestToolsPlugin(sdk)
	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From:    "file-tools",
		Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	}); err != nil {
		t.Fatal(err)
	}
	body := json.RawMessage(`{"name":"file_read","arguments":{"path":"README.md"},"session_id":"s1","workspace":"E:/project","workspace_id":"ws1","call_id":"c1","mode":"coding"}`)
	raw, err := p.call(context.Background(), &pluginsdk.Request{ContextID: "ctx-1", Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseResult(raw)
	if err != nil || parsed.Content != "hello" {
		t.Fatalf("result = %+v, %v", parsed, err)
	}
	if sdk.contextID != "ctx-1" || sdk.targetCap != "file" || sdk.targetMeth != "read" {
		t.Fatalf("target call = ctx=%s %s.%s", sdk.contextID, sdk.targetCap, sdk.targetMeth)
	}
	var provider ProviderInput
	if err := json.Unmarshal(sdk.callPayload, &provider); err != nil {
		t.Fatal(err)
	}
	if provider.SessionID != "s1" || provider.Workspace != "E:/project" ||
		provider.WorkspaceID != "ws1" || provider.CallID != "c1" || provider.Mode != "coding" {
		t.Fatalf("provider input = %+v", provider)
	}
}

func TestRegistryPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1)}
	p := &toolsPlugin{sdk: sdk, reg: newRegistry(), registryPath: path}
	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From:    "file-tools",
		Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("registry was not persisted: %v", err)
	}
	reg, err := loadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := &toolsPlugin{sdk: sdk, reg: reg, registryPath: path}
	out, err := reloaded.list(context.Background(), nil)
	if err != nil || len(out.Tools) != 1 || out.Tools[0].Name != "file_read" {
		t.Fatalf("reloaded list = %+v, %v", out, err)
	}
}

func TestCallRejectsStaleGeneration(t *testing.T) {
	result, _ := Success("hello", nil)
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1), target: result}
	p := newTestToolsPlugin(sdk)
	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From:    "file-tools",
		Payload: syncPayload(t, testDefinition("file_read", "file", "read", RiskLow)),
	}); err != nil {
		t.Fatal(err)
	}
	sdk.snapshot = activeSnapshot("file-tools", "file", "read", 2)
	body := json.RawMessage(`{"name":"file_read","arguments":{"path":"README.md"}}`)
	_, err := p.call(context.Background(), &pluginsdk.Request{Payload: body})
	if pluginsdk.Code(err) != "tool_target_unavailable" {
		t.Fatalf("err = %v", err)
	}
}

func TestSyncPreservesParallelSafe(t *testing.T) {
	sdk := &fakeToolsSDK{snapshot: activeSnapshot("file-tools", "file", "read", 1)}
	p := newTestToolsPlugin(sdk)
	def := testDefinition("file_read", "file", "read", RiskLow)
	def.ParallelSafe = true
	if _, err := p.sync(context.Background(), &pluginsdk.Request{
		From: "file-tools", Payload: syncPayload(t, def),
	}); err != nil {
		t.Fatal(err)
	}
	entry, ok := p.reg.get("file_read")
	if !ok || !entry.definition.ParallelSafe {
		t.Fatalf("parallel_safe was not preserved: %+v", entry)
	}
}
