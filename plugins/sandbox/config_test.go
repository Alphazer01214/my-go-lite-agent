package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"default_action":"deny","risk_bands":[{"max_risk":30,"action":"allow"},{"max_risk":100,"action":"deny"}],"rules_file":"policy.json"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := loadConfig(path)
	if cfg.DefaultAction != ActionDeny {
		t.Fatalf("default action = %s, want deny", cfg.DefaultAction)
	}
	if len(cfg.RiskBands) != 2 || cfg.RiskBands[0].MaxRisk != 30 {
		t.Fatalf("risk bands = %+v, want the file bands", cfg.RiskBands)
	}
	if cfg.RulesFile != "policy.json" {
		t.Fatalf("rules file = %s, want policy.json", cfg.RulesFile)
	}
}

func TestLoadConfigFallsBackOnABrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := loadConfig(path)
	if cfg.DefaultAction != defaultConfig().DefaultAction {
		t.Fatalf("default action = %s, want the default", cfg.DefaultAction)
	}
}

func TestLoadConfigRejectsAnInvalidBand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"risk_bands":[{"max_risk":300,"action":"allow"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := loadConfig(path)
	if len(cfg.RiskBands) != len(defaultConfig().RiskBands) {
		t.Fatalf("risk bands = %+v, want the default table", cfg.RiskBands)
	}
}

func TestLoadConfigKeepsTheRuleFileInsideThePluginDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"rules_file":"../escape.json"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := loadConfig(path)
	if cfg.RulesFile != defaultConfig().RulesFile {
		t.Fatalf("rules file = %s, want the default", cfg.RulesFile)
	}
}

func TestConfigSaveWritesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := defaultConfig()
	cfg.DefaultAction = ActionDeny
	if err := cfg.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded := loadConfig(path)
	if reloaded.DefaultAction != ActionDeny {
		t.Fatalf("default action = %s, want deny", reloaded.DefaultAction)
	}
}

func TestRuleStorePersistsTheRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	store := newRuleStore(path)
	ruleID, created, version, err := store.set(rule{
		Action: ActionDeny,
		Scope:  ruleScope{CommandPattern: "rm -rf *"},
		Note:   "no recursive delete",
	}, "")
	if err != nil {
		t.Fatalf("set rule: %v", err)
	}
	if !created || ruleID == "" || version == 0 {
		t.Fatalf("set rule result = %q %v %d, want a created rule", ruleID, created, version)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the rule file was not written: %v", err)
	}

	reloaded := newRuleStore(path)
	rules := reloaded.list(LifetimePersistent, "")
	if len(rules) != 1 || rules[0].RuleID != ruleID {
		t.Fatalf("reloaded rules = %+v, want the rule that was written", rules)
	}
	if rules[0].Note != "no recursive delete" {
		t.Fatalf("note = %q, want the note that was written", rules[0].Note)
	}
}

func TestRuleStoreSkipsABrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	store := newRuleStore(path)
	if rules := store.list(LifetimePersistent, ""); len(rules) != 0 {
		t.Fatalf("rules = %+v, want an empty rule set", rules)
	}
}

func TestRuleStoreKeepsSessionRulesInMemory(t *testing.T) {
	store := newRuleStore(filepath.Join(t.TempDir(), "rules.json"))
	if _, _, _, err := store.set(rule{
		Action:   ActionAllow,
		Scope:    ruleScope{Tool: "shell_execute"},
		Lifetime: LifetimeSession,
	}, "s-1"); err != nil {
		t.Fatalf("set session rule: %v", err)
	}
	if rules := store.list(LifetimeSession, "s-1"); len(rules) != 1 {
		t.Fatalf("session rules = %+v, want one rule", rules)
	}
	if rules := store.list(LifetimeSession, "s-2"); len(rules) != 0 {
		t.Fatalf("session rules of another session = %+v, want none", rules)
	}
}

func TestRuleStoreRefusesToDeleteADenyRule(t *testing.T) {
	store := newRuleStore(filepath.Join(t.TempDir(), "rules.json"))
	if _, _, _, err := store.set(rule{Action: ActionDeny, Scope: ruleScope{Tool: "file_write"}}, ""); err != nil {
		t.Fatalf("set deny rule: %v", err)
	}
	if _, err := store.delete("r-1", LifetimePersistent, ""); err == nil {
		t.Fatal("deleting a deny rule must be rejected")
	}
}

func TestRuleStoreFileKeepsTheVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	store := newRuleStore(path)
	if _, _, _, err := store.set(rule{Action: ActionAllow, Scope: ruleScope{Tool: "file_read"}}, ""); err != nil {
		t.Fatalf("set rule: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rules: %v", err)
	}
	var loaded persistedRules
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal rules: %v", err)
	}
	if loaded.Version == 0 || len(loaded.Rules) != 1 {
		t.Fatalf("rule file = %+v, want one rule and a version", loaded)
	}
}
