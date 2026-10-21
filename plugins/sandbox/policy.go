package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// decision is the outcome of one adjudication.
type decision struct {
	Action            string
	RuleID            string
	Reason            string
	Risk              int
	ModifiedArguments json.RawMessage
	Tool              string
	Capability        string
	Method            string
}

// evaluate decides one call. It is a pure function: it reads the rules and
// the configuration and returns a decision, without touching any state.
//
// The order is the one the design fixes:
//
//  1. a matching rule wins over the risk bands;
//  2. among matching rules the strongest action wins, then the weight, then
//     the narrower scope, then the rule id;
//  3. without a matching rule the risk band decides;
//  4. without a covering band the default action decides.
func evaluate(rules []rule, cfg config, call callContext, risk int, now time.Time) decision {
	matched := make([]rule, 0, 4)
	for _, item := range rules {
		if expired(item, now) {
			continue
		}
		if item.Scope.matches(call) {
			matched = append(matched, item)
		}
	}
	if len(matched) > 0 {
		sortRules(matched)
		best := matched[0]
		action := best.Action
		reason := ruleReason(best)
		// A modify rule without a template cannot rewrite anything. It is
		// broken, so it denies instead of silently passing the call through.
		if action == ActionModify && len(best.ModifiedArguments) == 0 {
			action = ActionDeny
			reason = fmt.Sprintf("rule %s has no modified_arguments, deny instead of rewriting", best.RuleID)
		}
		return decision{
			Action:            action,
			RuleID:            best.RuleID,
			Reason:            reason,
			Risk:              risk,
			ModifiedArguments: best.ModifiedArguments,
			Tool:              call.Tool,
			Capability:        call.Capability,
			Method:            call.Method,
		}
	}
	action := cfg.DefaultAction
	reason := fmt.Sprintf("no rule matched, risk %d, default action %s", risk, action)
	if bandAction, ok := cfg.bandAction(risk); ok {
		low, high, _ := cfg.bandRange(risk)
		action = bandAction
		reason = fmt.Sprintf("risk %d falls in band %d-%d, action %s", risk, low, high, action)
	}
	return decision{
		Action:     action,
		Reason:     reason,
		Risk:       risk,
		Tool:       call.Tool,
		Capability: call.Capability,
		Method:     call.Method,
	}
}

// ruleReason renders the reason of a rule hit.
func ruleReason(item rule) string {
	reason := fmt.Sprintf("matched rule %s (%s)", item.RuleID, describeScope(item.Scope))
	if item.Note != "" {
		reason += ": " + item.Note
	}
	return reason
}
