package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type fakeSandboxSDK struct {
	tools []toolcontract.Definition
}

func (f *fakeSandboxSDK) Call(_ context.Context, capability, method string, _ json.RawMessage) (json.RawMessage, error) {
	if capability == "tools" && method == "list" {
		return json.Marshal(listOut{Tools: f.tools})
	}
	return nil, pluginsdk.ErrCode(pluginsdk.CodeMethodNotFound, "unexpected call")
}

func (f *fakeSandboxSDK) CallInContext(ctx context.Context, _, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	return f.Call(ctx, capability, method, payload)
}

func TestDefaultPolicyByRisk(t *testing.T) {
	store := newPolicyStore(filepath.Join(t.TempDir(), "missing.json"))
	if got := store.decide(ruleContext{Capability: "file", Method: "read", Risk: "low"}); !got.Allowed {
		t.Fatalf("low = %+v", got)
	}
	if got := store.decide(ruleContext{Capability: "web", Method: "fetch", Risk: "medium"}); !got.Allowed {
		t.Fatalf("medium = %+v", got)
	}
	if got := store.decide(ruleContext{Capability: "shell", Method: "execute", Risk: "high"}); got.Allowed {
		t.Fatalf("high = %+v", got)
	}
}

func TestHighRiskRuleByWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "permissions.json")
	workspace := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(workspace, 0o755)
	body := `{"rules":[{"capability":"file","method":"write","action":"allow","workspace_prefix":"` +
		filepath.ToSlash(workspace) + `"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newPolicyStore(path)
	allowed := store.decide(ruleContext{
		Capability: "file", Method: "write", Risk: "high", Workspace: filepath.Join(workspace, "sub"),
	})
	if !allowed.Allowed {
		t.Fatalf("inside = %+v", allowed)
	}
	denied := store.decide(ruleContext{
		Capability: "file", Method: "write", Risk: "high", Workspace: filepath.Join(dir, "elsewhere"),
	})
	if denied.Allowed {
		t.Fatalf("outside = %+v", denied)
	}
}

func TestToolCallUsesTargetRisk(t *testing.T) {
	sdk := &fakeSandboxSDK{tools: []toolcontract.Definition{{
		Name: "file_write", Risk: "low", TargetCapability: "file", TargetMethod: "write",
	}}}
	p := newSandbox(sdk)
	p.policy = newPolicyStore(filepath.Join(t.TempDir(), "missing.json"))
	req := &pluginsdk.Request{
		Capability: "tools", Method: "call", ContextID: "ctx-1",
		Payload: json.RawMessage(`{"name":"file_write","arguments":{},"workspace":"E:/project"}`),
	}
	got, err := p.authorize(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Allowed || got.Risk != "high" {
		t.Fatalf("decision = %+v", got)
	}
}

func TestUnknownTargetIsDenied(t *testing.T) {
	p := newSandbox(&fakeSandboxSDK{})
	got, err := p.authorize(context.Background(), &pluginsdk.Request{
		Capability: "unknown", Method: "run", Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Allowed {
		t.Fatalf("decision = %+v", got)
	}
}
