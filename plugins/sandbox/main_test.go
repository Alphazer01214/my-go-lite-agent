package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// fakeCaller is a stand-in for the SDK. It answers tools.list and
// session.append and records what it was asked.
type fakeCaller struct {
	mu        sync.Mutex
	tools     []toolEntry
	listErr   error
	appendErr error
	appends   []fakeAppend
	calls     []string
}

// fakeAppend is one recorded session.append call.
type fakeAppend struct {
	SessionID string
	Facts     []map[string]any
}

func (f *fakeCaller) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	key := capability + "." + method
	f.mu.Lock()
	f.calls = append(f.calls, key)
	f.mu.Unlock()
	switch key {
	case "tools.list":
		if f.listErr != nil {
			return nil, f.listErr
		}
		f.mu.Lock()
		tools := append([]toolEntry(nil), f.tools...)
		f.mu.Unlock()
		return mustJSON(map[string]any{"tools": tools}), nil
	case "session.append":
		if f.appendErr != nil {
			return nil, f.appendErr
		}
		var in struct {
			SessionID string           `json:"session_id"`
			Facts     []map[string]any `json:"facts"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		f.mu.Lock()
		f.appends = append(f.appends, fakeAppend{SessionID: in.SessionID, Facts: in.Facts})
		f.mu.Unlock()
		return mustJSON(map[string]any{"session_id": in.SessionID, "next_seq": 1}), nil
	default:
		return nil, fmt.Errorf("unexpected call %s", key)
	}
}

// appends returns the recorded session writes.
func (f *fakeCaller) appends_() []fakeAppend {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeAppend(nil), f.appends...)
}

// newTestPlugin builds a plugin that reads no file from the working
// directory.
func newTestPlugin(t *testing.T, caller sdkCaller) *plugin {
	t.Helper()
	dir := t.TempDir()
	return &plugin{
		sdk:        caller,
		audit:      &auditLog{limit: auditRingLimit},
		cfg:        defaultConfig(),
		configPath: filepath.Join(dir, "config.json"),
		rules:      newRuleStore(filepath.Join(dir, "rules.json")),
		pending:    newPendingStore(),
		approved:   newApprovalStore(),
		uiRev:      1,
	}
}

// testTools is the registry of a typical deployment.
func testTools() []toolEntry {
	return []toolEntry{
		{Name: "file_read", Risk: 10, TargetCapability: "file", TargetMethod: "read"},
		{Name: "file_write", Risk: 80, TargetCapability: "file", TargetMethod: "write"},
		{Name: "file_patch", Risk: 80, TargetCapability: "file", TargetMethod: "patch"},
		{Name: "shell_execute", Risk: 90, TargetCapability: "shell", TargetMethod: "execute"},
	}
}

func TestSeamAllowsAReadOnlyCall(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	payload := json.RawMessage(`{"name":"file_read","arguments":{"path":"README.md"},"session_id":"s-1","call_id":"c-1"}`)
	req := &pluginsdk.Request{Capability: "tools", Method: "call", Payload: payload}

	var proceeded json.RawMessage
	out, err := p.adjudicate(context.Background(), req, seamPayload{
		Name:      "file_read",
		Arguments: json.RawMessage(`{"path":"README.md"}`),
		SessionID: "s-1",
		CallID:    "c-1",
	}, func(_ context.Context, body json.RawMessage) (json.RawMessage, error) {
		proceeded = body
		return json.RawMessage(`{"content":"ok"}`), nil
	})
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if string(proceeded) != string(payload) {
		t.Fatalf("proceeded payload = %s, want the original payload", proceeded)
	}
	if string(out) != `{"content":"ok"}` {
		t.Fatalf("result = %s, want the downstream result", out)
	}
}

func TestSeamDeniesAWriteAndReturnsAToolResult(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	if _, _, _, err := p.rules.set(rule{
		Action: ActionDeny,
		Scope:  ruleScope{Tool: "file_write"},
	}, ""); err != nil {
		t.Fatalf("set deny rule: %v", err)
	}
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"file_write","arguments":{"path":"a.txt"},"session_id":"s-1"}`)}

	proceeded := false
	out, err := p.adjudicate(context.Background(), req, seamPayload{
		Name:      "file_write",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		SessionID: "s-1",
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		proceeded = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if proceeded {
		t.Fatal("a denied call must not continue the pipeline")
	}
	var result toolResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.IsError {
		t.Fatal("a denied call must return an error result")
	}
	if result.Meta["code"] != CodeDenied {
		t.Fatalf("meta code = %v, want %s", result.Meta["code"], CodeDenied)
	}
	if result.Meta["sandbox_decision"] != ActionDeny {
		t.Fatalf("meta decision = %v, want deny", result.Meta["sandbox_decision"])
	}
	if result.Meta["risk"] != float64(80) {
		t.Fatalf("meta risk = %v, want 80", result.Meta["risk"])
	}
}

func TestSeamModifiesTheArguments(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	if _, _, _, err := p.rules.set(rule{
		Action:            ActionModify,
		Scope:             ruleScope{Tool: "file_write"},
		ModifiedArguments: json.RawMessage(`{"overwrite":false}`),
	}, ""); err != nil {
		t.Fatalf("set rule: %v", err)
	}
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"file_write","arguments":{"path":"a.txt","content":"x"},"session_id":"s-1"}`)}

	var proceeded json.RawMessage
	if _, err := p.adjudicate(context.Background(), req, seamPayload{
		Name:      "file_write",
		Arguments: json.RawMessage(`{"path":"a.txt","content":"x"}`),
		SessionID: "s-1",
	}, func(_ context.Context, body json.RawMessage) (json.RawMessage, error) {
		proceeded = body
		return json.RawMessage(`{"content":"ok"}`), nil
	}); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	var forwarded struct {
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(proceeded, &forwarded); err != nil {
		t.Fatalf("unmarshal forwarded payload: %v", err)
	}
	if forwarded.Arguments["overwrite"] != false {
		t.Fatalf("arguments = %v, want overwrite false", forwarded.Arguments)
	}
	if forwarded.Arguments["content"] != "x" {
		t.Fatalf("arguments = %v, want the original content kept", forwarded.Arguments)
	}
}

func TestSeamAdjudicatesADirectProviderCall(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	if _, _, _, err := p.rules.set(rule{
		Action: ActionDeny,
		Scope:  ruleScope{CommandPattern: "rm -rf *"},
	}, ""); err != nil {
		t.Fatalf("set rule: %v", err)
	}
	req := &pluginsdk.Request{Capability: "shell", Method: "execute",
		Payload: json.RawMessage(`{"arguments":{"command":"rm","args":["-rf","build"]},"session_id":"s-1"}`)}

	proceeded := false
	out, err := p.adjudicate(context.Background(), req, seamPayload{
		Arguments: json.RawMessage(`{"command":"rm","args":["-rf","build"]}`),
		SessionID: "s-1",
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		proceeded = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if proceeded {
		t.Fatal("a denied direct call must not continue the pipeline")
	}
	var result toolResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Meta["code"] != CodeDenied {
		t.Fatalf("meta code = %v, want %s", result.Meta["code"], CodeDenied)
	}
	if result.Meta["tool"] != "shell_execute" {
		t.Fatalf("meta tool = %v, want shell_execute", result.Meta["tool"])
	}
}

func TestSeamWritesTheAuditEntryToTheSession(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"file_read","arguments":{"path":"a.txt"},"session_id":"s-1","call_id":"c-9"}`)}

	if _, err := p.adjudicate(context.Background(), req, seamPayload{
		Name:      "file_read",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		SessionID: "s-1",
		CallID:    "c-9",
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	appends := caller.appends_()
	if len(appends) != 1 {
		t.Fatalf("session writes = %d, want 1", len(appends))
	}
	fact := appends[0].Facts[0]
	if fact["type"] != FactTypeDecision {
		t.Fatalf("fact type = %v, want %s", fact["type"], FactTypeDecision)
	}
	meta, _ := fact["meta"].(map[string]any)
	if meta["decision"] != ActionAllow {
		t.Fatalf("meta decision = %v, want allow", meta["decision"])
	}
	if meta["session_id"] != "s-1" {
		t.Fatalf("meta session id = %v, want s-1", meta["session_id"])
	}
	if meta["call_id"] != "c-9" {
		t.Fatalf("meta call id = %v, want c-9", meta["call_id"])
	}
	if digest, _ := meta["arguments_digest"].(string); len(digest) < 8 {
		t.Fatalf("meta arguments digest = %q, want a digest", digest)
	}
}

func TestAuditFallsBackToTheRingBufferWithoutASession(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"file_read","arguments":{"path":"a.txt"}}`)}

	if _, err := p.adjudicate(context.Background(), req, seamPayload{
		Name:      "file_read",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if len(caller.appends_()) != 0 {
		t.Fatal("without a session id nothing is written to the session")
	}
	entries := p.audit.entries()
	if len(entries) != 1 {
		t.Fatalf("ring buffer entries = %d, want 1", len(entries))
	}
	if entries[0].Decision != ActionAllow {
		t.Fatalf("entry decision = %s, want allow", entries[0].Decision)
	}
}

func TestAuditSurvivesAFailingSession(t *testing.T) {
	caller := &fakeCaller{tools: testTools(), appendErr: errors.New("session down")}
	p := newTestPlugin(t, caller)
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"file_read","arguments":{"path":"a.txt"},"session_id":"s-1"}`)}

	if _, err := p.adjudicate(context.Background(), req, seamPayload{
		Name:      "file_read",
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		SessionID: "s-1",
	}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}); err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if len(p.audit.entries()) != 1 {
		t.Fatal("a failing session write must fall back to the ring buffer")
	}
}

func TestUnknownToolIsJudgedWithTheUnknownRisk(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	p.cfg.RiskBands = []band{{MaxRisk: 49, Action: ActionAllow}, {MaxRisk: 100, Action: ActionDeny}}
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"mystery_tool","arguments":{}}`)}

	out, err := p.adjudicate(context.Background(), req, seamPayload{Name: "mystery_tool"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		})
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	var result toolResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Meta["risk"] != float64(UnknownRisk) {
		t.Fatalf("meta risk = %v, want %d", result.Meta["risk"], UnknownRisk)
	}
}

func TestAFailingToolRegistryIsJudgedWithTheUnknownRisk(t *testing.T) {
	caller := &fakeCaller{listErr: errors.New("tools down")}
	p := newTestPlugin(t, caller)
	p.cfg.RiskBands = []band{{MaxRisk: 49, Action: ActionAllow}, {MaxRisk: 100, Action: ActionDeny}}
	req := &pluginsdk.Request{Capability: "tools", Method: "call",
		Payload: json.RawMessage(`{"name":"file_read","arguments":{}}`)}

	out, err := p.adjudicate(context.Background(), req, seamPayload{Name: "file_read"},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		})
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	var result toolResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Meta["risk"] != float64(UnknownRisk) {
		t.Fatalf("meta risk = %v, want %d", result.Meta["risk"], UnknownRisk)
	}
}

func TestCheckIsDry(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	out, err := p.check(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"tool":"shell_execute","arguments":{"command":"rm -rf build"}}`),
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if out.Decision != ActionConfirm {
		t.Fatalf("decision = %s, want confirm", out.Decision)
	}
	if len(caller.appends_()) != 0 {
		t.Fatal("a dry run must not write an audit entry")
	}
}

func TestCheckRejectsAMissingTarget(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	_, err := p.check(context.Background(), &pluginsdk.Request{Payload: json.RawMessage(`{}`)})
	if pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("error code = %q, want %q", pluginsdk.Code(err), pluginsdk.CodeBadArguments)
	}
}

func TestSetRuleRejectsAWeakeningChange(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	if _, _, _, err := p.rules.set(rule{
		Action: ActionDeny,
		Scope:  ruleScope{PathGlob: "/ws/**"},
	}, ""); err != nil {
		t.Fatalf("set deny rule: %v", err)
	}
	_, err := p.setRule(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"action":"allow","scope":{"path_glob":"/ws/**"}}`),
	})
	if pluginsdk.Code(err) != CodeRuleInvalid {
		t.Fatalf("error code = %q, want %q", pluginsdk.Code(err), CodeRuleInvalid)
	}
}

func TestSetRuleReportsAMissingRule(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	_, err := p.setRule(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"rule_id":"r-404","delete":true}`),
	})
	if pluginsdk.Code(err) != CodeRuleNotFound {
		t.Fatalf("error code = %q, want %q", pluginsdk.Code(err), CodeRuleNotFound)
	}
}

func TestSetRuleAddsAndListsARule(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	out, err := p.setRule(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"action":"deny","scope":{"command_pattern":"rm -rf *"},"note":"no recursive delete"}`),
	})
	if err != nil {
		t.Fatalf("set rule: %v", err)
	}
	if !out.Created || out.RuleID == "" {
		t.Fatalf("set rule result = %+v, want a created rule with an id", out)
	}
	list, err := p.listRules(context.Background(), &pluginsdk.Request{Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(list.Rules) != 1 || list.Rules[0].RuleID != out.RuleID {
		t.Fatalf("rules = %+v, want the rule that was just added", list.Rules)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	got, err := p.settingsGet(context.Background(), &pluginsdk.Request{Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("settings get: %v", err)
	}
	if got.Values["default_action"] != ActionAllow {
		t.Fatalf("default action = %v, want allow", got.Values["default_action"])
	}
	if _, err := p.settingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"default_action":"deny"}}`),
	}); err != nil {
		t.Fatalf("settings save: %v", err)
	}
	if p.cfg.DefaultAction != ActionDeny {
		t.Fatalf("default action = %s, want deny", p.cfg.DefaultAction)
	}
}

func TestSettingsRejectAnInvalidValue(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	_, err := p.settingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"default_action":"bogus"}}`),
	})
	if pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("error code = %q, want %q", pluginsdk.Code(err), pluginsdk.CodeBadArguments)
	}
}
