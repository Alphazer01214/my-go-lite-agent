package main

import "testing"

func TestEvaluateRulePrecedence(t *testing.T) {
	rules := []rule{
		{RuleID: "allow", Action: ActionAllow, Priority: 100, Scope: ruleScope{Capability: "shell", Method: "execute"}},
		{RuleID: "confirm", Action: ActionConfirm, Priority: 1, Scope: ruleScope{Capability: "shell", Method: "execute"}},
	}
	got := evaluate(rules, ActionAllow, callContext{Capability: "shell", Method: "execute"})
	if got.Action != ActionConfirm || got.RuleID != "confirm" {
		t.Fatalf("decision = %+v", got)
	}
}

func TestEvaluatePriorityAndSpecificity(t *testing.T) {
	rules := []rule{
		{RuleID: "broad", Action: ActionConfirm, Priority: 10, Scope: ruleScope{Capability: "file"}},
		{RuleID: "narrow", Action: ActionConfirm, Priority: 10, Scope: ruleScope{Capability: "file", Method: "write"}},
	}
	got := evaluate(rules, ActionAllow, callContext{Capability: "file", Method: "write"})
	if got.RuleID != "narrow" {
		t.Fatalf("rule = %q", got.RuleID)
	}
}

func TestEvaluateDefaultAction(t *testing.T) {
	got := evaluate(nil, ActionAllow, callContext{Capability: "file", Method: "write"})
	if got.Action != ActionAllow || got.RuleID != "" {
		t.Fatalf("decision = %+v", got)
	}
}

func TestEvaluateModifyWithoutTemplateDenies(t *testing.T) {
	rules := []rule{{
		RuleID: "broken",
		Action: ActionModify,
		Scope:  ruleScope{Capability: "file", Method: "write"},
	}}
	got := evaluate(rules, ActionAllow, callContext{Capability: "file", Method: "write"})
	if got.Action != ActionDeny {
		t.Fatalf("decision = %+v", got)
	}
}

func TestScopeConjunction(t *testing.T) {
	scope := ruleScope{Capability: "file", Method: "write", PathGlob: "**/*.env"}
	if !scope.matches(callContext{Capability: "file", Method: "write", Path: "config/.env", Workspace: "E:/ws"}) {
		t.Fatal("scope should match")
	}
	if scope.matches(callContext{Capability: "file", Method: "write", Path: "config/app.json", Workspace: "E:/ws"}) {
		t.Fatal("path condition should reject")
	}
	if scope.matches(callContext{Capability: "shell", Method: "execute", Path: "config/.env", Workspace: "E:/ws"}) {
		t.Fatal("capability condition should reject")
	}
}

func TestEmptyScopeNeverMatches(t *testing.T) {
	if (ruleScope{}).matches(callContext{Capability: "file", Method: "write"}) {
		t.Fatal("empty scope must not match")
	}
}
