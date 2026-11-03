package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestConfirmTimeoutReturnsExpiredResult(t *testing.T) {
	cfg := defaultConfig()
	cfg.ConfirmTTLSeconds = 1
	cfg.Rules = []rule{{
		RuleID: "r-1",
		Action: ActionConfirm,
		Scope:  ruleScope{Capability: "shell", Method: "execute"},
	}}
	p, _ := newTestPlugin(t, cfg)
	raw := json.RawMessage(`{"arguments":{"command":"rm","args":["-rf","build"]}}`)
	var payload seamPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	req := &pluginsdk.Request{Capability: "shell", Method: "execute", Payload: raw}
	out, err := p.adjudicate(context.Background(), req, payload, func(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
		t.Fatal("expired confirmation must not proceed")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var result toolResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Meta["code"] != CodeConfirmExpired {
		t.Fatalf("result = %+v", result)
	}
	if p.pending.count() != 0 {
		t.Fatal("expired token must be removed")
	}
}

func TestCheckReturnsDecision(t *testing.T) {
	cfg := defaultConfig()
	cfg.Rules = []rule{{
		RuleID: "r-1",
		Action: ActionDeny,
		Scope:  ruleScope{Tool: "file_write", PathGlob: "**/.env"},
	}}
	p, _ := newTestPlugin(t, cfg)
	out, err := p.check(context.Background(), &checkIn{
		Tool:      "file_write",
		Arguments: json.RawMessage(`{"path":".env","content":"x"}`),
		Workspace: "E:/ws",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != ActionDeny || out.RuleID != "r-1" {
		t.Fatalf("check = %+v", out)
	}
}

func TestCheckRejectsMissingTarget(t *testing.T) {
	p, _ := newTestPlugin(t, defaultConfig())
	if _, err := p.check(context.Background(), &checkIn{}); err == nil {
		t.Fatal("missing target must fail")
	}
}
