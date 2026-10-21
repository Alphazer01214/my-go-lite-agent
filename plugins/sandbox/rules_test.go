package main

import "testing"

func TestScopeMatchesIsAConjunction(t *testing.T) {
	call := callContext{
		Tool:       "file_write",
		Capability: "file",
		Method:     "write",
		Path:       "src/main.go",
		Workspace:  "/ws",
	}
	cases := []struct {
		name  string
		scope ruleScope
		want  bool
	}{
		{"tool only", ruleScope{Tool: "file_write"}, true},
		{"tool mismatch", ruleScope{Tool: "file_read"}, false},
		{"capability and method", ruleScope{Capability: "file", Method: "write"}, true},
		{"method mismatch", ruleScope{Method: "read"}, false},
		{"path glob", ruleScope{PathGlob: "src/**"}, true},
		{"path glob mismatch", ruleScope{PathGlob: "docs/**"}, false},
		{"all conditions hold", ruleScope{Tool: "file_write", PathGlob: "src/**"}, true},
		{"one condition fails", ruleScope{Tool: "file_write", PathGlob: "docs/**"}, false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := item.scope.matches(call); got != item.want {
				t.Fatalf("matches = %v, want %v", got, item.want)
			}
		})
	}
}

func TestScopeWithNoConditionNeverMatches(t *testing.T) {
	if (ruleScope{}).matches(callContext{Tool: "file_write"}) {
		t.Fatal("a scope without a condition must not match")
	}
}

func TestCompareRulesOrdersByActionThenWeightThenScope(t *testing.T) {
	deny := rule{RuleID: "r-1", Action: ActionDeny, Priority: 1, Scope: ruleScope{Tool: "file_write"}}
	allow := rule{RuleID: "r-2", Action: ActionAllow, Priority: 999, Scope: ruleScope{Tool: "file_write"}}
	if compareRules(deny, allow) >= 0 {
		t.Fatal("deny must outrank allow regardless of weight")
	}
	heavy := rule{RuleID: "r-3", Action: ActionDeny, Priority: 10, Scope: ruleScope{Tool: "file_write"}}
	light := rule{RuleID: "r-4", Action: ActionDeny, Priority: 5, Scope: ruleScope{Tool: "file_write"}}
	if compareRules(heavy, light) >= 0 {
		t.Fatal("the heavier rule must come first")
	}
	narrow := rule{RuleID: "r-5", Action: ActionDeny, Priority: 5, Scope: ruleScope{Tool: "file_write", PathGlob: "src/**"}}
	wide := rule{RuleID: "r-6", Action: ActionDeny, Priority: 5, Scope: ruleScope{Tool: "file_write"}}
	if compareRules(narrow, wide) >= 0 {
		t.Fatal("the narrower scope must come first")
	}
	first := rule{RuleID: "r-7", Action: ActionDeny, Priority: 5, Scope: ruleScope{Tool: "file_write"}}
	second := rule{RuleID: "r-8", Action: ActionDeny, Priority: 5, Scope: ruleScope{Tool: "file_write"}}
	if compareRules(first, second) >= 0 {
		t.Fatal("the smaller rule id must come first")
	}
}

func TestWeakenDetectsABroaderAllow(t *testing.T) {
	deny := rule{RuleID: "r-1", Action: ActionDeny, Scope: ruleScope{PathGlob: "/ws/**"}}
	broaderAllow := rule{RuleID: "r-2", Action: ActionAllow, Scope: ruleScope{PathGlob: "/ws/**"}}
	if !weakens([]rule{deny}, broaderAllow) {
		t.Fatal("an allow rule over a deny rule weakens the set")
	}
	narrowerAllow := rule{RuleID: "r-3", Action: ActionAllow, Scope: ruleScope{PathGlob: "/ws/src/**"}}
	if weakens([]rule{deny}, narrowerAllow) {
		t.Fatal("a narrower allow rule does not weaken the set")
	}
}

func TestWeakenDetectsABroaderScopeWithTheSameAction(t *testing.T) {
	allow := rule{RuleID: "r-1", Action: ActionAllow, Scope: ruleScope{PathGlob: "/ws/src/**"}}
	broader := rule{RuleID: "r-2", Action: ActionAllow, Scope: ruleScope{PathGlob: "/ws/**"}}
	if !weakens([]rule{allow}, broader) {
		t.Fatal("the same action over a broader scope weakens the set")
	}
}

func TestWeakenIgnoresUnrelatedScopes(t *testing.T) {
	allow := rule{RuleID: "r-1", Action: ActionAllow, Scope: ruleScope{Tool: "file_write"}}
	other := rule{RuleID: "r-2", Action: ActionAllow, Scope: ruleScope{Tool: "shell_execute"}}
	if weakens([]rule{allow}, other) {
		t.Fatal("rules over different tools do not weaken each other")
	}
	deny := rule{RuleID: "r-3", Action: ActionDeny, Scope: ruleScope{Tool: "shell_execute"}}
	if weakens([]rule{allow}, deny) {
		t.Fatal("a stricter rule never weakens the set")
	}
}

func TestScopeContains(t *testing.T) {
	outer := ruleScope{PathGlob: "/ws/**"}
	inner := ruleScope{PathGlob: "/ws/src/**"}
	if !scopeContains(outer, inner) {
		t.Fatal("a broader glob must cover a narrower one")
	}
	if scopeContains(inner, outer) {
		t.Fatal("a narrower glob must not cover a broader one")
	}
	if !scopeContains(ruleScope{}, inner) {
		t.Fatal("an empty scope covers everything")
	}
	if scopeContains(inner, ruleScope{}) {
		t.Fatal("a constrained scope does not cover an unconstrained one")
	}
}
