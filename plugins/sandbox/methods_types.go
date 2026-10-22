package main

import (
	"encoding/json"
)

type checkIn struct {
	Tool       string          `json:"tool,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Capability string          `json:"capability,omitempty"`
	Method     string          `json:"method,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	Workspace  string          `json:"workspace,omitempty"`
}

type checkOut struct {
	Decision          string          `json:"decision"`
	RuleID            string          `json:"rule_id"`
	Reason            string          `json:"reason"`
	ModifiedArguments json.RawMessage `json:"modified_arguments"`
	Risk              int             `json:"risk"`
}

type listRulesIn struct {
	Lifetime  string `json:"lifetime,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

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

type listRulesOut struct {
	Rules []ruleView `json:"rules"`
}

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

type setRuleOut struct {
	RuleID       string `json:"rule_id"`
	Created      bool   `json:"created"`
	RulesVersion int    `json:"rules_version"`
}

type settingsGetOut struct {
	Values map[string]any `json:"values"`
}

type settingsSaveIn struct {
	Values struct {
		DefaultAction      *string `json:"default_action,omitempty"`
		AuditRetentionDays *int    `json:"audit_retention_days,omitempty"`
	} `json:"values"`
}

type settingsSaveOut struct {
	Saved bool `json:"saved"`
}
