package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// rulesFromSettings converts the flat editor rows into rules. Blank rows are
// dropped; anything else must be a complete, well-formed rule.
func rulesFromSettings(items []ruleSetting) ([]rule, error) {
	out := make([]rule, 0, len(items))
	for _, item := range items {
		if emptyRuleSetting(item) {
			continue
		}
		if item.Action == "" {
			return nil, ruleInvalid("action is required")
		}
		scope := ruleScope{
			Tool:           strings.TrimSpace(item.Tool),
			Capability:     strings.TrimSpace(item.Capability),
			Method:         strings.TrimSpace(item.Method),
			PathGlob:       strings.TrimSpace(item.PathGlob),
			CommandPattern: strings.TrimSpace(item.CommandPattern),
		}
		if scope.empty() {
			return nil, ruleInvalid("scope must name at least one condition")
		}
		modified, err := parseModifiedArguments(item.ModifiedArguments)
		if err != nil {
			return nil, err
		}
		if item.Action == ActionModify && len(modified) == 0 {
			return nil, ruleInvalid("modified_arguments is required for a modify rule")
		}
		out = append(out, rule{
			RuleID:            strings.TrimSpace(item.RuleID),
			Action:            item.Action,
			Scope:             scope,
			Priority:          item.Priority,
			Lifetime:          LifetimePersistent,
			TTLSeconds:        item.TTLSeconds,
			Note:              item.Note,
			ModifiedArguments: modified,
		})
	}
	return out, nil
}

func emptyRuleSetting(item ruleSetting) bool {
	return item.RuleID == "" && item.Action == "" && item.Tool == "" &&
		item.Capability == "" && item.Method == "" && item.PathGlob == "" &&
		item.CommandPattern == "" && item.Priority == 0 && item.TTLSeconds == 0 &&
		item.Note == "" && item.ModifiedArguments == ""
}

// parseModifiedArguments accepts the editor's JSON text for a modify template.
// An empty string means there is no template.
func parseModifiedArguments(raw string) (json.RawMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
		return nil, ruleInvalid("modified_arguments must be a JSON object")
	}
	return json.RawMessage(raw), nil
}

// compactJSON renders a raw JSON value on one line for the editor. The editor
// stores it as text, so formatting noise only makes the row harder to read.
func compactJSON(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}
