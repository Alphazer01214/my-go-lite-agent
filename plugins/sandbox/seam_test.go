package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func testProceed(captured *json.RawMessage) seamProceed {
	return func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
		*captured = append(json.RawMessage(nil), payload...)
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func TestAdjudicateAllowsByDefault(t *testing.T) {
	p, _ := newTestPlugin(t, defaultConfig())
	raw := json.RawMessage(`{"arguments":{"path":"src/main.go","content":"x"}}`)
	var payload seamPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	req := &pluginsdk.Request{Capability: "file", Method: "write", Payload: raw}
	var proceeded json.RawMessage
	out, err := p.adjudicate(context.Background(), req, payload, testProceed(&proceeded))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"ok":true}` {
		t.Fatalf("out = %s", out)
	}
	if string(proceeded) != string(raw) {
		t.Fatalf("proceeded payload = %s", proceeded)
	}
}

func TestAdjudicateDeniesMatchingRule(t *testing.T) {
	cfg := defaultConfig()
	cfg.Rules = []rule{{
		RuleID: "r-1",
		Action: ActionDeny,
		Scope:  ruleScope{Capability: "file", Method: "write"},
	}}
	p, _ := newTestPlugin(t, cfg)
	raw := json.RawMessage(`{"arguments":{"path":"src/main.go","content":"x"}}`)
	var payload seamPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	req := &pluginsdk.Request{Capability: "file", Method: "write", Payload: raw}
	var proceeded json.RawMessage
	out, err := p.adjudicate(context.Background(), req, payload, testProceed(&proceeded))
	if err != nil {
		t.Fatal(err)
	}
	if proceeded != nil {
		t.Fatal("denied call must not proceed")
	}
	var result toolResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Meta["code"] != CodeDenied {
		t.Fatalf("result = %+v", result)
	}
}

func TestAdjudicateModifiesArguments(t *testing.T) {
	cfg := defaultConfig()
	cfg.Rules = []rule{{
		RuleID:            "r-1",
		Action:            ActionModify,
		Scope:             ruleScope{Capability: "file", Method: "write"},
		ModifiedArguments: json.RawMessage(`{"path":"safe.txt"}`),
	}}
	p, _ := newTestPlugin(t, cfg)
	raw := json.RawMessage(`{"arguments":{"path":"danger.txt","content":"x"}}`)
	var payload seamPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	req := &pluginsdk.Request{Capability: "file", Method: "write", Payload: raw}
	var proceeded json.RawMessage
	if _, err := p.adjudicate(context.Background(), req, payload, testProceed(&proceeded)); err != nil {
		t.Fatal(err)
	}
	var rewritten seamPayload
	if err := json.Unmarshal(proceeded, &rewritten); err != nil {
		t.Fatal(err)
	}
	var args map[string]string
	if err := json.Unmarshal(rewritten.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args["path"] != "safe.txt" || args["content"] != "x" {
		t.Fatalf("rewritten args = %#v", args)
	}
}

func TestConfirmAllowProceedsAndMarksNestedSeam(t *testing.T) {
	cfg := defaultConfig()
	cfg.ConfirmTTLSeconds = 300
	cfg.Rules = []rule{{
		RuleID: "r-1",
		Action: ActionConfirm,
		Scope:  ruleScope{Capability: "file", Method: "write"},
	}}
	p, _ := newTestPlugin(t, cfg)

	outerRaw := json.RawMessage(`{"name":"file_write","arguments":{"path":"a.txt","content":"x"},"call_id":"call-1"}`)
	var outer seamPayload
	if err := json.Unmarshal(outerRaw, &outer); err != nil {
		t.Fatal(err)
	}
	outerReq := &pluginsdk.Request{Capability: "tool", Method: "call", Payload: outerRaw}
	var outerProceeded json.RawMessage
	done := make(chan error, 1)
	go func() {
		_, err := p.adjudicate(context.Background(), outerReq, outer, testProceed(&outerProceeded))
		done <- err
	}()

	token := waitForPendingToken(t, p)
	if _, err := p.pending.decide(token, ActionAllow, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if outerProceeded == nil {
		t.Fatal("confirmed call must proceed")
	}

	// The dispatched provider seam carries the same call_id. It must not ask a
	// second time even though the rule would match again.
	innerRaw := json.RawMessage(`{"arguments":{"path":"a.txt","content":"x"},"call_id":"call-1"}`)
	var inner seamPayload
	if err := json.Unmarshal(innerRaw, &inner); err != nil {
		t.Fatal(err)
	}
	innerReq := &pluginsdk.Request{Capability: "file", Method: "write", Payload: innerRaw}
	var innerProceeded json.RawMessage
	if _, err := p.adjudicate(context.Background(), innerReq, inner, testProceed(&innerProceeded)); err != nil {
		t.Fatal(err)
	}
	if innerProceeded == nil {
		t.Fatal("nested seam must skip duplicate adjudication")
	}
	if p.pending.count() != 0 {
		t.Fatal("nested seam must not create another pending confirmation")
	}
}

func waitForPendingToken(t *testing.T, p *plugin) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		pending := p.pending.list("", time.Now())
		if len(pending) > 0 {
			return pending[0].Token
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for pending confirmation")
	return ""
}
