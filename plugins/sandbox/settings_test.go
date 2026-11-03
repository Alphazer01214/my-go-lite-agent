package main

import (
	"encoding/json"
	"testing"
)

func TestSettingsRulesRoundTrip(t *testing.T) {
	rules := []rule{{
		RuleID:   "r-7",
		Action:   ActionModify,
		Priority: 12,
		Note:     "rewrite",
		Scope: ruleScope{
			Capability:     "file",
			Method:         "write",
			PathGlob:       "**/.env",
			CommandPattern: "git *",
		},
		ModifiedArguments: json.RawMessage(`{"path":"safe.env"}`),
	}}
	flat := rulesToSettings(rules)
	if len(flat) != 1 || flat[0].ModifiedArguments != `{"path":"safe.env"}` {
		t.Fatalf("flat = %+v", flat)
	}
	back := rulesFromSettings(flat)
	if len(back) != 1 {
		t.Fatalf("back = %+v", back)
	}
	got := back[0]
	if got.RuleID != "r-7" || got.Action != ActionModify || got.Priority != 12 {
		t.Fatalf("rule = %+v", got)
	}
	if got.Scope != rules[0].Scope {
		t.Fatalf("scope = %+v", got.Scope)
	}
}

func TestSettingsDropsBlankRows(t *testing.T) {
	rules := rulesFromSettings([]ruleSetting{{}})
	if len(rules) != 0 {
		t.Fatalf("rules = %+v", rules)
	}
}
