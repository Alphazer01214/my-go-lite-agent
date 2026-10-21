package main

import (
	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// ruleInvalid reports a rule that breaks the schema or the monotonicity
// constraint of the rule set.
func ruleInvalid(message string) error {
	return pluginsdk.ErrCode(CodeRuleInvalid, message)
}

// ruleNotFound reports an update or a delete that names no known rule.
func ruleNotFound(message string) error {
	return pluginsdk.ErrCode(CodeRuleNotFound, message)
}
