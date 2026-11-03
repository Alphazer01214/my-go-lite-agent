package main

import (
	"encoding/json"
	"fmt"
)

type decision struct {
	Action            string
	RuleID            string
	Reason            string
	ModifiedArguments json.RawMessage
	Tool              string
	Capability        string
	Method            string
}

// evaluate is pure: a matching rule wins over the default action; among
// matching rules the strongest action wins, then priority, then the narrower
// scope, then rule id.
func evaluate(rules []rule, defaultAction string, call callContext) decision {
	matched := make([]rule, 0, len(rules))
	for _, item := range rules {
		if item.Scope.matches(call) {
			matched = append(matched, item)
		}
	}
	if len(matched) > 0 {
		sortRules(matched)
		best := matched[0]
		action := best.Action
		reason := fmt.Sprintf("matched rule %s", best.RuleID)
		if best.Note != "" {
			reason += ": " + best.Note
		}
		// A modify rule without a template cannot rewrite anything, so it
		// fails closed instead of silently passing the call through.
		if action == ActionModify && len(best.ModifiedArguments) == 0 {
			action = ActionDeny
			reason = fmt.Sprintf("rule %s has no modified_arguments, deny instead of rewriting", best.RuleID)
		}
		return decision{
			Action:            action,
			RuleID:            best.RuleID,
			Reason:            reason,
			ModifiedArguments: best.ModifiedArguments,
			Tool:              call.Tool,
			Capability:        call.Capability,
			Method:            call.Method,
		}
	}
	return decision{
		Action:     defaultAction,
		Reason:     fmt.Sprintf("no rule matched, default action %s", defaultAction),
		Tool:       call.Tool,
		Capability: call.Capability,
		Method:     call.Method,
	}
}

func (p *plugin) decide(call callContext) decision {
	p.mu.RLock()
	cfg := p.cfg
	rules := append([]rule(nil), cfg.Rules...)
	p.mu.RUnlock()
	return evaluate(rules, cfg.DefaultAction, call)
}

func ruleCount(rules []rule) int {
	return len(rules)
}
