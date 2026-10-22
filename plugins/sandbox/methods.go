package main

import (
	"context"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

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

func (p *plugin) settingsGet(_ context.Context, _ *pluginsdk.Request) (settingsGetOut, error) {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()
	return settingsGetOut{Values: map[string]any{
		"default_action":       cfg.DefaultAction,
		"audit_retention_days": cfg.AuditRetentionDays,
	}}, nil
}

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
