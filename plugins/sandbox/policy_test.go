package main

import (
	"testing"
	"time"
)

func TestEvaluateUsesTheRiskBandWithoutAMatchingRule(t *testing.T) {
	cfg := defaultConfig()
	call := callContext{Tool: "file_read", Capability: "file", Method: "read"}
	verdict := evaluate(nil, cfg, call, 10, time.Now())
	if verdict.Action != ActionAllow {
		t.Fatalf("action = %s, want allow", verdict.Action)
	}
	if verdict.RuleID != "" {
		t.Fatalf("rule id = %q, want empty", verdict.RuleID)
	}
}

func TestEvaluateDeniesAWriteWithoutAMatchingRule(t *testing.T) {
	cfg := defaultConfig()
	call := callContext{Tool: "file_write", Capability: "file", Method: "write"}
	verdict := evaluate(nil, cfg, call, 80, time.Now())
	if verdict.Action != ActionDeny {
		t.Fatalf("action = %s, want deny", verdict.Action)
	}
}

func TestEvaluateUsesTheDefaultActionWhenNoBandCovers(t *testing.T) {
	cfg := defaultConfig()
	cfg.RiskBands = []band{{MaxRisk: 20, Action: ActionAllow}}
	cfg.DefaultAction = ActionDeny
	call := callContext{Tool: "file_read"}
	verdict := evaluate(nil, cfg, call, 90, time.Now())
	if verdict.Action != ActionDeny {
		t.Fatalf("action = %s, want deny", verdict.Action)
	}
}

func TestEvaluatePrefersTheRuleOverTheBand(t *testing.T) {
	cfg := defaultConfig()
	rules := []rule{{
		RuleID: "r-1",
		Action: ActionAllow,
		Scope:  ruleScope{Tool: "file_write"},
	}}
	call := callContext{Tool: "file_write", Capability: "file", Method: "write"}
	verdict := evaluate(rules, cfg, call, 80, time.Now())
	if verdict.Action != ActionAllow {
		t.Fatalf("action = %s, want allow", verdict.Action)
	}
	if verdict.RuleID != "r-1" {
		t.Fatalf("rule id = %q, want r-1", verdict.RuleID)
	}
}

func TestEvaluatePrefersTheStrongestAction(t *testing.T) {
	cfg := defaultConfig()
	rules := []rule{
		{RuleID: "r-1", Action: ActionAllow, Scope: ruleScope{Tool: "file_write"}},
		{RuleID: "r-2", Action: ActionDeny, Scope: ruleScope{Capability: "file"}},
		{RuleID: "r-3", Action: ActionModify, Scope: ruleScope{Method: "write"},
			ModifiedArguments: []byte(`{"overwrite":false}`)},
	}
	call := callContext{Tool: "file_write", Capability: "file", Method: "write"}
	verdict := evaluate(rules, cfg, call, 80, time.Now())
	if verdict.Action != ActionDeny {
		t.Fatalf("action = %s, want deny", verdict.Action)
	}
}

func TestEvaluateDeniesAModifyRuleWithoutATemplate(t *testing.T) {
	cfg := defaultConfig()
	rules := []rule{{
		RuleID: "r-1",
		Action: ActionModify,
		Scope:  ruleScope{Tool: "file_write"},
	}}
	call := callContext{Tool: "file_write", Capability: "file", Method: "write"}
	verdict := evaluate(rules, cfg, call, 80, time.Now())
	if verdict.Action != ActionDeny {
		t.Fatalf("action = %s, want deny", verdict.Action)
	}
}

func TestEvaluateSkipsAnExpiredRule(t *testing.T) {
	cfg := defaultConfig()
	rules := []rule{{
		RuleID:     "r-1",
		Action:     ActionAllow,
		Scope:      ruleScope{Tool: "file_write"},
		TTLSeconds: 60,
		CreatedTS:  time.Now().Add(-2 * time.Minute).UnixMilli(),
	}}
	call := callContext{Tool: "file_write", Capability: "file", Method: "write"}
	verdict := evaluate(rules, cfg, call, 80, time.Now())
	if verdict.Action != ActionDeny {
		t.Fatalf("action = %s, want deny from the risk band", verdict.Action)
	}
}
