package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestSettingsRuleRoundTrip(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	body := `{"values":{
		"risk_bands":[{"max_risk":49,"action":"allow"},{"max_risk":100,"action":"confirm"}],
		"confirm_ttl_seconds":120,
		"rules":[
			{"action":"confirm","tool":"file_write","note":"ask before writing"},
			{"rule_id":"r-9","action":"deny","command_pattern":"rm -rf *","priority":50}
		]
	}}`
	if _, err := p.settingsSave(context.Background(), &pluginsdk.Request{Payload: json.RawMessage(body)}); err != nil {
		t.Fatalf("settings save: %v", err)
	}
	if p.cfg.ConfirmTTLSeconds != 120 {
		t.Fatalf("ttl = %d, want 120", p.cfg.ConfirmTTLSeconds)
	}
	if len(p.cfg.RiskBands) != 2 || p.cfg.RiskBands[1].Action != ActionConfirm {
		t.Fatalf("bands = %+v", p.cfg.RiskBands)
	}
	rules := p.rules.list(LifetimePersistent, "")
	if len(rules) != 2 {
		t.Fatalf("rules = %+v", rules)
	}
	byID := map[string]rule{}
	for _, item := range rules {
		byID[item.RuleID] = item
	}
	if item := byID["r-9"]; item.Action != ActionDeny || item.Scope.CommandPattern != "rm -rf *" || item.Priority != 50 {
		t.Fatalf("r-9 = %+v", item)
	}
	confirm := rules[0]
	if confirm.RuleID == "" || confirm.RuleID == "r-9" {
		confirm = rules[1]
	}
	if confirm.Action != ActionConfirm || confirm.Scope.Tool != "file_write" || confirm.Note != "ask before writing" {
		t.Fatalf("confirm rule = %+v", confirm)
	}

	got, err := p.settingsGet(context.Background(), &pluginsdk.Request{Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("settings get: %v", err)
	}
	if got.Values["confirm_ttl_seconds"] != 120 {
		t.Fatalf("ttl value = %v", got.Values["confirm_ttl_seconds"])
	}
	views, ok := got.Values["rules"].([]ruleSetting)
	if !ok || len(views) != 2 {
		t.Fatalf("rule views = %#v", got.Values["rules"])
	}
	var sawConfirm bool
	for _, view := range views {
		if view.Tool == "file_write" && view.Action == ActionConfirm {
			sawConfirm = true
		}
	}
	if !sawConfirm {
		t.Fatalf("rule views = %+v", views)
	}
}

func TestSettingsCanDeleteADenyRule(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	if _, _, _, err := p.rules.set(rule{
		Action: ActionDeny, Scope: ruleScope{Tool: "file_write"},
	}, ""); err != nil {
		t.Fatalf("set deny rule: %v", err)
	}
	if _, err := p.settingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"rules":[]}}`),
	}); err != nil {
		t.Fatalf("settings save: %v", err)
	}
	if rules := p.rules.list(LifetimePersistent, ""); len(rules) != 0 {
		t.Fatalf("rules = %+v, want the deny rule removed", rules)
	}
}

func TestSettingsRejectAnInvalidRule(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	_, err := p.settingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"rules":[{"action":"allow"}]}}`),
	})
	if pluginsdk.Code(err) != CodeRuleInvalid {
		t.Fatalf("error = %v, want sandbox_rule_invalid", err)
	}
	if rules := p.rules.list(LifetimePersistent, ""); len(rules) != 0 {
		t.Fatalf("rules changed after a rejected save: %+v", rules)
	}
}

func TestSettingsRejectModifyAsAnOutcome(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	_, err := p.settingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"risk_bands":[{"max_risk":100,"action":"modify"}]}}`),
	})
	if pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("error = %v, want bad_arguments", err)
	}
}

func TestSettingsRuleModifiedArgumentsMustBeAnObject(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	_, err := p.settingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"rules":[{"action":"modify","tool":"file_write","modified_arguments":"[1]"}]}}`),
	})
	if pluginsdk.Code(err) != CodeRuleInvalid {
		t.Fatalf("error = %v, want sandbox_rule_invalid", err)
	}
}
