package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// checkIn is the payload of sandbox.check.
type checkIn struct {
	Tool       string          `json:"tool,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Capability string          `json:"capability,omitempty"`
	Method     string          `json:"method,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
}

// checkOut is the payload of sandbox.check.
type checkOut struct {
	Decision          string          `json:"decision"`
	RuleID            string          `json:"rule_id"`
	Reason            string          `json:"reason"`
	ModifiedArguments json.RawMessage `json:"modified_arguments"`
	Risk              int             `json:"risk"`
}

// listRulesIn is the payload of sandbox.list_rules.
type listRulesIn struct {
	Lifetime  string `json:"lifetime,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// ruleView is one rule as the list method returns it.
type ruleView struct {
	RuleID     string    `json:"rule_id"`
	Action     string    `json:"action"`
	Scope      ruleScope `json:"scope"`
	Priority   int       `json:"priority"`
	Lifetime   string    `json:"lifetime"`
	TTLSeconds int       `json:"ttl_seconds"`
	Note       string    `json:"note,omitempty"`
	CreatedTS  int64     `json:"created_ts"`
}

// listRulesOut is the payload of sandbox.list_rules.
type listRulesOut struct {
	Rules []ruleView `json:"rules"`
}

// setRuleIn is the payload of sandbox.set_rule.
//
// Delete is an extension of the target contract: the contract fixes the
// behaviour of deleting a rule but has no method for it, so the delete rides
// on the same method.
type setRuleIn struct {
	RuleID            string          `json:"rule_id,omitempty"`
	Action            string          `json:"action,omitempty"`
	Scope             ruleScope       `json:"scope,omitempty"`
	Priority          int             `json:"priority,omitempty"`
	Lifetime          string          `json:"lifetime,omitempty"`
	TTLSeconds        int             `json:"ttl_seconds,omitempty"`
	Note              string          `json:"note,omitempty"`
	ModifiedArguments json.RawMessage `json:"modified_arguments,omitempty"`
	SessionID         string          `json:"session_id,omitempty"`
	Delete            bool            `json:"delete,omitempty"`
}

// setRuleOut is the payload of sandbox.set_rule.
type setRuleOut struct {
	RuleID       string `json:"rule_id"`
	Created      bool   `json:"created"`
	RulesVersion int    `json:"rules_version"`
}

// settingsGetOut is the payload of sandbox.settings.get.
type settingsGetOut struct {
	Values map[string]any `json:"values"`
}

// settingsSaveIn is the payload of sandbox.settings.save.
type settingsSaveIn struct {
	Values struct {
		DefaultAction      *string `json:"default_action,omitempty"`
		AuditRetentionDays *int    `json:"audit_retention_days,omitempty"`
	} `json:"values"`
}

// settingsSaveOut is the payload of sandbox.settings.save.
type settingsSaveOut struct {
	Saved bool `json:"saved"`
}

// check judges one candidate call without side effects: it writes no audit
// entry and changes no rule.
func (p *plugin) check(ctx context.Context, req *pluginsdk.Request) (checkOut, error) {
	var in checkIn
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return checkOut{}, err
	}
	if in.Tool == "" && (in.Capability == "" || in.Method == "") {
		return checkOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
			"tool or capability and method are required")
	}

	table := p.toolTable(ctx)
	call := callContext{SessionID: in.SessionID, Workspace: in.Workspace}
	risk := UnknownRisk

	if in.Tool != "" {
		call.Tool = in.Tool
		entry, known := table.lookup(in.Tool)
		if !known {
			call.Capability = in.Capability
			call.Method = in.Method
		} else {
			if in.Capability != "" && in.Capability != entry.TargetCapability {
				return checkOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
					"tool and capability name different targets")
			}
			if in.Method != "" && in.Method != entry.TargetMethod {
				return checkOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
					"tool and method name different targets")
			}
			call.Capability = entry.TargetCapability
			call.Method = entry.TargetMethod
			risk = entry.Risk
		}
	} else {
		call.Capability = in.Capability
		call.Method = in.Method
		if entry, ok := table.lookupTarget(in.Capability, in.Method); ok {
			call.Tool = entry.Name
			risk = entry.Risk
		}
	}

	args, err := Arguments(in.Arguments)
	if err != nil {
		return checkOut{}, err
	}
	switch call.Capability {
	case "file":
		call.Path = ArgumentString(args, "path")
	case "shell":
		call.Command = commandLine(args)
	}

	verdict := p.decide(call, risk)
	out := checkOut{
		Decision: verdict.Action,
		RuleID:   verdict.RuleID,
		Reason:   verdict.Reason,
		Risk:     verdict.Risk,
	}
	if verdict.Action == ActionModify {
		out.ModifiedArguments = verdict.ModifiedArguments
	}
	return out, nil
}

// listRules returns the rules in effect.
func (p *plugin) listRules(_ context.Context, req *pluginsdk.Request) (listRulesOut, error) {
	var in listRulesIn
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return listRulesOut{}, err
	}
	lifetime := in.Lifetime
	if lifetime == "" {
		lifetime = "all"
	}
	switch lifetime {
	case "all", LifetimePersistent, LifetimeSession:
	default:
		return listRulesOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
			"lifetime must be all, persistent or session")
	}
	if lifetime == LifetimeSession && in.SessionID == "" {
		return listRulesOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
			"session_id is required for session rules")
	}
	rules := p.rules.list(lifetime, in.SessionID)
	sortForList(rules)
	views := make([]ruleView, 0, len(rules))
	for _, item := range rules {
		views = append(views, ruleView{
			RuleID:     item.RuleID,
			Action:     item.Action,
			Scope:      item.Scope,
			Priority:   item.Priority,
			Lifetime:   item.Lifetime,
			TTLSeconds: item.TTLSeconds,
			Note:       item.Note,
			CreatedTS:  item.CreatedTS,
		})
	}
	return listRulesOut{Rules: views}, nil
}

// setRule adds, updates or deletes one rule.
func (p *plugin) setRule(_ context.Context, req *pluginsdk.Request) (setRuleOut, error) {
	var in setRuleIn
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return setRuleOut{}, err
	}
	lifetime := in.Lifetime
	if lifetime == "" {
		lifetime = LifetimePersistent
	}
	if in.Delete {
		if in.RuleID == "" {
			return setRuleOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
				"rule_id is required to delete a rule")
		}
		version, err := p.rules.delete(in.RuleID, lifetime, in.SessionID)
		if err != nil {
			return setRuleOut{}, err
		}
		return setRuleOut{RuleID: in.RuleID, Created: false, RulesVersion: version}, nil
	}

	next := rule{
		RuleID:            in.RuleID,
		Action:            in.Action,
		Scope:             in.Scope,
		Priority:          in.Priority,
		Lifetime:          lifetime,
		TTLSeconds:        in.TTLSeconds,
		Note:              in.Note,
		ModifiedArguments: in.ModifiedArguments,
	}
	ruleID, created, version, err := p.rules.set(next, in.SessionID)
	if err != nil {
		return setRuleOut{}, err
	}
	return setRuleOut{RuleID: ruleID, Created: created, RulesVersion: version}, nil
}

// settingsGet reads the settings.
func (p *plugin) settingsGet(_ context.Context, _ *pluginsdk.Request) (settingsGetOut, error) {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()
	return settingsGetOut{Values: map[string]any{
		"default_action":       cfg.DefaultAction,
		"audit_retention_days": cfg.AuditRetentionDays,
	}}, nil
}

// settingsSave writes the settings. A rejected value is reported and nothing
// is written.
func (p *plugin) settingsSave(_ context.Context, req *pluginsdk.Request) (settingsSaveOut, error) {
	var in settingsSaveIn
	if err := DecodeStrict(req.Payload, &in); err != nil {
		return settingsSaveOut{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	next := p.cfg
	if in.Values.DefaultAction != nil {
		if !validAction(*in.Values.DefaultAction) {
			return settingsSaveOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
				"default_action must be allow or deny")
		}
		next.DefaultAction = *in.Values.DefaultAction
	}
	if in.Values.AuditRetentionDays != nil {
		if *in.Values.AuditRetentionDays < 1 || *in.Values.AuditRetentionDays > 3650 {
			return settingsSaveOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments,
				"audit_retention_days must be between 1 and 3650")
		}
		next.AuditRetentionDays = *in.Values.AuditRetentionDays
	}
	if err := next.save(p.configPath); err != nil {
		return settingsSaveOut{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	p.cfg = next
	return settingsSaveOut{Saved: true}, nil
}
