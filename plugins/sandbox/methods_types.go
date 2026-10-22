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

// ruleSetting is the flat form of a rule used by the settings editor. Scope
// conditions are lifted to the top level so the Host list widget can render
// them without nested objects.
type ruleSetting struct {
	RuleID            string `json:"rule_id,omitempty"`
	Action            string `json:"action,omitempty"`
	Tool              string `json:"tool,omitempty"`
	Capability        string `json:"capability,omitempty"`
	Method            string `json:"method,omitempty"`
	PathGlob          string `json:"path_glob,omitempty"`
	CommandPattern    string `json:"command_pattern,omitempty"`
	Priority          int    `json:"priority,omitempty"`
	TTLSeconds        int    `json:"ttl_seconds,omitempty"`
	Note              string `json:"note,omitempty"`
	ModifiedArguments string `json:"modified_arguments,omitempty"`
}

type settingsSaveIn struct {
	Values struct {
		DefaultAction      *string        `json:"default_action,omitempty"`
		ConfirmTTLSeconds  *int           `json:"confirm_ttl_seconds,omitempty"`
		AuditRetentionDays *int           `json:"audit_retention_days,omitempty"`
		RiskBands          *[]band        `json:"risk_bands,omitempty"`
		Rules              *[]ruleSetting `json:"rules,omitempty"`
	} `json:"values"`
}

type settingsSaveOut struct {
	Saved bool `json:"saved"`
}

type pendingView struct {
	Token            string `json:"token"`
	Tool             string `json:"tool"`
	Capability       string `json:"capability,omitempty"`
	Method           string `json:"method,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	RuleID           string `json:"rule_id,omitempty"`
	Reason           string `json:"reason"`
	Risk             int    `json:"risk"`
	ArgumentsPreview string `json:"arguments_preview,omitempty"`
	CreatedTS        int64  `json:"created_ts"`
	ExpiresTS        int64  `json:"expires_at"`
}

type pendingListIn struct {
	SessionID string `json:"session_id,omitempty"`
}

type pendingListOut struct {
	Pending []pendingView `json:"pending"`
}

type decideIn struct {
	Token    string `json:"token"`
	Decision string `json:"decision"`
}

type decideOut struct {
	Token     string `json:"token"`
	Decision  string `json:"decision"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expires_at"`
}

type confirmPolicyUI struct {
	DefaultAction string `json:"default_action"`
	RuleCount     int    `json:"rule_count"`
	PendingCount  int    `json:"pending_count"`
}

type confirmStateUI struct {
	Revision int             `json:"revision"`
	Pending  []pendingView   `json:"pending"`
	Policy   confirmPolicyUI `json:"policy"`
}
