package main

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	ActionAllow   = "allow"
	ActionConfirm = "confirm"
	ActionModify  = "modify"
	ActionDeny    = "deny"
)

func actionRank(action string) int {
	switch action {
	case ActionDeny:
		return 4
	case ActionConfirm:
		return 3
	case ActionModify:
		return 2
	case ActionAllow:
		return 1
	default:
		return 0
	}
}

func validAction(action string) bool {
	return action == ActionAllow || action == ActionConfirm ||
		action == ActionModify || action == ActionDeny
}

func validOutcomeAction(action string) bool {
	return action == ActionAllow || action == ActionConfirm || action == ActionDeny
}

type ruleScope struct {
	Tool           string `json:"tool,omitempty"`
	Capability     string `json:"capability,omitempty"`
	Method         string `json:"method,omitempty"`
	PathGlob       string `json:"path_glob,omitempty"`
	CommandPattern string `json:"command_pattern,omitempty"`
}

type rule struct {
	RuleID            string          `json:"rule_id"`
	Action            string          `json:"action"`
	Scope             ruleScope       `json:"scope"`
	Priority          int             `json:"priority,omitempty"`
	Note              string          `json:"note,omitempty"`
	ModifiedArguments json.RawMessage `json:"modified_arguments,omitempty"`
	CreatedTS         int64           `json:"created_ts,omitempty"`
}

type callContext struct {
	Tool       string
	Capability string
	Method     string
	Path       string
	Command    string
	Workspace  string
	SessionID  string
	CallID     string
}

func (s ruleScope) matches(ctx callContext) bool {
	if s.empty() {
		return false
	}
	if s.Tool != "" && s.Tool != ctx.Tool {
		return false
	}
	if s.Capability != "" && s.Capability != ctx.Capability {
		return false
	}
	if s.Method != "" && s.Method != ctx.Method {
		return false
	}
	if s.PathGlob != "" && !matchPath(s.PathGlob, ctx.Path, ctx.Workspace) {
		return false
	}
	if s.CommandPattern != "" && !matchCommand(s.CommandPattern, ctx.Command) {
		return false
	}
	return true
}

func (s ruleScope) empty() bool {
	return s.Tool == "" && s.Capability == "" && s.Method == "" &&
		s.PathGlob == "" && s.CommandPattern == ""
}

func (s ruleScope) specificity() int {
	score := 0
	for _, value := range []string{s.Tool, s.Capability, s.Method} {
		if value != "" {
			score += 2
		}
	}
	if s.PathGlob != "" {
		score++
	}
	if s.CommandPattern != "" {
		score++
	}
	return score
}

func compareRules(left, right rule) int {
	if l, r := actionRank(left.Action), actionRank(right.Action); l != r {
		if l > r {
			return -1
		}
		return 1
	}
	if left.Priority != right.Priority {
		if left.Priority > right.Priority {
			return -1
		}
		return 1
	}
	if l, r := left.Scope.specificity(), right.Scope.specificity(); l != r {
		if l > r {
			return -1
		}
		return 1
	}
	return strings.Compare(left.RuleID, right.RuleID)
}

func sortRules(rules []rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return compareRules(rules[i], rules[j]) < 0
	})
}
