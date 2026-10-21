package main

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

// Actions a rule can carry. confirm is a target-contract state
// (docs/design/plugins/08-sandbox.md §1) and lands with the confirmation
// flow; this build accepts allow, modify and deny only.
const (
	ActionAllow  = "allow"
	ActionModify = "modify"
	ActionDeny   = "deny"
)

// Lifetimes a rule can carry.
const (
	LifetimePersistent = "persistent"
	LifetimeSession    = "session"
)

// actionRank orders the actions. A higher rank wins when several rules match
// the same call.
func actionRank(action string) int {
	switch action {
	case ActionDeny:
		return 3
	case ActionModify:
		return 2
	case ActionAllow:
		return 1
	default:
		return 0
	}
}

// validAction reports whether an action exists in this build.
func validAction(action string) bool {
	return action == ActionAllow || action == ActionModify || action == ActionDeny
}

// ruleScope is the match condition of a rule. A rule with several conditions
// is a conjunction: every condition must hold for the rule to match.
type ruleScope struct {
	Tool           string `json:"tool,omitempty"`
	Capability     string `json:"capability,omitempty"`
	Method         string `json:"method,omitempty"`
	PathGlob       string `json:"path_glob,omitempty"`
	CommandPattern string `json:"command_pattern,omitempty"`
}

// rule is one adjudication rule.
type rule struct {
	RuleID            string          `json:"rule_id"`
	Action            string          `json:"action"`
	Scope             ruleScope       `json:"scope"`
	Priority          int             `json:"priority"`
	Lifetime          string          `json:"lifetime"`
	TTLSeconds        int             `json:"ttl_seconds"`
	Note              string          `json:"note,omitempty"`
	ModifiedArguments json.RawMessage `json:"modified_arguments,omitempty"`
	CreatedTS         int64           `json:"created_ts"`
}

// callContext is the flattened view a rule is matched against.
type callContext struct {
	Tool       string
	Capability string
	Method     string
	Path       string
	Command    string
	Workspace  string
	SessionID  string
	CallID     string
	TurnID     string
	StepID     string
}

// matches reports whether every condition of the scope holds for the call.
// A scope with no condition never matches: a rule without a condition would
// otherwise swallow every call.
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

// empty reports whether a scope names no condition at all.
func (s ruleScope) empty() bool {
	return s.Tool == "" && s.Capability == "" && s.Method == "" &&
		s.PathGlob == "" && s.CommandPattern == ""
}

// specificity scores how narrow a scope is. It breaks ties between rules with
// the same action and the same weight: the narrower scope wins.
//
// The score is a proxy, not a containment proof: a literal condition counts
// double, a pattern condition counts once.
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

// compareRules orders two matching rules. The order is the one the design
// fixes: action first, then weight, then scope, then rule id.
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

// sortRules orders a rule list by the same rule set.
func sortRules(rules []rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return compareRules(rules[i], rules[j]) < 0
	})
}

// scopeContains reports whether outer covers inner: every condition inner
// constrains is covered by outer.
//
// A side that constrains nothing covers the other side, so an empty scope is
// the broadest one. The comparison of two constrained sides is a conservative
// approximation:
//
//   - a literal covers only the same literal;
//   - a pattern covers a literal when the pattern matches it;
//   - two patterns are compared by their literal prefixes.
func scopeContains(outer, inner ruleScope) bool {
	if !valueCovers(outer.Tool, inner.Tool) {
		return false
	}
	if !valueCovers(outer.Capability, inner.Capability) {
		return false
	}
	if !valueCovers(outer.Method, inner.Method) {
		return false
	}
	if !pathCovers(outer.PathGlob, inner.PathGlob) {
		return false
	}
	return commandCovers(outer.CommandPattern, inner.CommandPattern)
}

// valueCovers reports whether an outer literal or pattern covers an inner one.
func valueCovers(outer, inner string) bool {
	if outer == "" && inner == "" {
		return true
	}
	if inner == "" {
		// The inner constrains nothing: a constrained outer does not cover it.
		return false
	}
	if outer == "" {
		// The outer constrains nothing: it covers every inner.
		return true
	}
	if outer == inner {
		return true
	}
	return hasWildcard(outer)
}

// pathCovers reports whether an outer path glob covers an inner one.
func pathCovers(outer, inner string) bool {
	if outer == "" && inner == "" {
		return true
	}
	if inner == "" {
		return false
	}
	if outer == "" {
		return true
	}
	if outer == inner {
		return true
	}
	if !hasWildcard(inner) {
		return matchSegments(strings.Split(filepath.ToSlash(outer), "/"), strings.Split(filepath.ToSlash(inner), "/"))
	}
	if !hasWildcard(outer) {
		return false
	}
	outerPrefix, innerPrefix := literalPrefix(outer), literalPrefix(inner)
	return len(outerPrefix) <= len(innerPrefix) && strings.HasPrefix(innerPrefix, outerPrefix)
}

// commandCovers reports whether an outer command pattern covers an inner one.
func commandCovers(outer, inner string) bool {
	if outer == "" && inner == "" {
		return true
	}
	if inner == "" {
		return false
	}
	if outer == "" {
		return true
	}
	if outer == inner {
		return true
	}
	if !hasWildcard(inner) {
		return matchCommand(outer, inner)
	}
	if !hasWildcard(outer) {
		return false
	}
	outerPrefix, innerPrefix := literalPrefix(outer), literalPrefix(inner)
	return len(outerPrefix) <= len(innerPrefix) && strings.HasPrefix(innerPrefix, outerPrefix)
}

// literalPrefix returns the leading part of a pattern that holds no wildcard.
func literalPrefix(pattern string) string {
	if index := strings.IndexAny(pattern, "*?"); index >= 0 {
		return pattern[:index]
	}
	return pattern
}

// hasWildcard reports whether a pattern uses a wildcard.
func hasWildcard(value string) bool {
	return strings.ContainsAny(value, "*?")
}

// weakens reports whether adding or updating a rule would make the rule set
// weaker than it is now.
//
// A rule set is monotonic: it only moves towards stricter. The check is:
//
//   - a new rule with a weaker action over a scope that covers an existing
//     stricter rule weakens the set;
//   - a new rule with the same action over a strictly broader scope weakens
//     the set.
//
// Incomparable scopes do not weaken each other, so unrelated rules stay
// addable.
func weakens(existing []rule, next rule) bool {
	for _, other := range existing {
		if other.RuleID == next.RuleID {
			continue
		}
		if other.Action == next.Action {
			if scopeContains(next.Scope, other.Scope) && !scopeContains(other.Scope, next.Scope) {
				return true
			}
			continue
		}
		if actionRank(other.Action) > actionRank(next.Action) &&
			scopeContains(next.Scope, other.Scope) {
			return true
		}
	}
	return false
}
