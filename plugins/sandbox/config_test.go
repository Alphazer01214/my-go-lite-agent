package main

import (
	"encoding/json"
	"testing"
)

func TestConfigNormalizeDefaultsRuleActionAndID(t *testing.T) {
	cfg := config{
		Rules: []rule{{
			Scope: ruleScope{Capability: "shell", Method: "execute"},
		}},
	}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultAction != ActionAllow || cfg.ConfirmTTLSeconds != defaultConfirmTTL {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Rules[0].Action != ActionConfirm {
		t.Fatalf("action = %q", cfg.Rules[0].Action)
	}
	if cfg.Rules[0].RuleID != "r-1" {
		t.Fatalf("rule id = %q", cfg.Rules[0].RuleID)
	}
}

func TestConfigRejectsInvalidDefaultAction(t *testing.T) {
	cfg := config{DefaultAction: ActionModify, ConfirmTTLSeconds: 300}
	if err := cfg.normalize(); err == nil {
		t.Fatal("modify must not be a default outcome")
	}
}

func TestConfigRejectsEmptyScope(t *testing.T) {
	cfg := config{
		DefaultAction:     ActionAllow,
		ConfirmTTLSeconds: 300,
		Rules:             []rule{{Action: ActionDeny}},
	}
	if err := cfg.normalize(); err == nil {
		t.Fatal("empty scope must fail")
	}
}

func TestConfigRejectsModifyWithoutTemplate(t *testing.T) {
	cfg := config{
		DefaultAction:     ActionAllow,
		ConfirmTTLSeconds: 300,
		Rules: []rule{{
			Action: ActionModify,
			Scope:  ruleScope{Capability: "file", Method: "write"},
		}},
	}
	if err := cfg.normalize(); err == nil {
		t.Fatal("modify without template must fail")
	}
}

func TestConfigNormalizesModifyTemplate(t *testing.T) {
	cfg := config{
		DefaultAction:     ActionAllow,
		ConfirmTTLSeconds: 300,
		Rules: []rule{{
			Action:            ActionModify,
			Scope:             ruleScope{Capability: "file", Method: "write"},
			ModifiedArguments: json.RawMessage(` { "path" : "safe.txt" } `),
		}},
	}
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	var args map[string]string
	if err := json.Unmarshal(cfg.Rules[0].ModifiedArguments, &args); err != nil {
		t.Fatal(err)
	}
	if args["path"] != "safe.txt" {
		t.Fatalf("args = %#v", args)
	}
}
