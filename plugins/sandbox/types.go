package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	CodeDenied         = "sandbox_denied"
	CodeConfirmExpired = "sandbox_confirm_expired"
	CodeRuleInvalid    = "sandbox_rule_invalid"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type uiPublisher interface {
	SetState(mountID string, state json.RawMessage) error
}

type plugin struct {
	sdk        sdkCaller
	ui         uiPublisher
	configPath string

	mu  sync.RWMutex
	cfg config

	audit    *auditLog
	pending  *pendingStore
	approved *approvalStore
	uiRev    uint64

	toolMu sync.Mutex
	tools  toolIndex
}

func newPlugin(sdk sdkCaller, cfg config, configPath string) *plugin {
	return &plugin{
		sdk:        sdk,
		configPath: configPath,
		cfg:        cfg,
		audit:      &auditLog{limit: auditRingLimit},
		pending:    newPendingStore(),
		approved:   newApprovalStore(),
		uiRev:      uint64(time.Now().UnixMicro()),
	}
}

func (p *plugin) currentConfig() config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

func (p *plugin) setConfig(cfg config) {
	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()
}

type pendingConfirm struct {
	Token            string
	Tool             string
	Capability       string
	Method           string
	SessionID        string
	RuleID           string
	Reason           string
	ArgumentsPreview string
	ArgumentsDigest  string
	CreatedTS        int64
	ExpiresTS        int64
	Decision         chan string
}

type pendingView struct {
	Token            string `json:"token"`
	Tool             string `json:"tool,omitempty"`
	Capability       string `json:"capability,omitempty"`
	Method           string `json:"method,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	RuleID           string `json:"rule_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ArgumentsPreview string `json:"arguments_preview,omitempty"`
	CreatedTS        int64  `json:"created_ts"`
	ExpiresAt        int64  `json:"expires_at"`
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

type ruleSetting struct {
	RuleID            string `json:"rule_id,omitempty"`
	Action            string `json:"action,omitempty"`
	Tool              string `json:"tool,omitempty"`
	Capability        string `json:"capability,omitempty"`
	Method            string `json:"method,omitempty"`
	PathGlob          string `json:"path_glob,omitempty"`
	CommandPattern    string `json:"command_pattern,omitempty"`
	Priority          int    `json:"priority,omitempty"`
	Note              string `json:"note,omitempty"`
	ModifiedArguments string `json:"modified_arguments,omitempty"`
}

type settingsValues struct {
	DefaultAction     string        `json:"default_action"`
	ConfirmTTLSeconds int           `json:"confirm_ttl_seconds"`
	Rules             []ruleSetting `json:"rules"`
}

type settingsGetResp struct {
	Values settingsValues `json:"values"`
}

type settingsSaveReq struct {
	Values settingsValues `json:"values"`
}

type settingsSaveResp struct {
	Saved bool `json:"saved"`
}

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
	RuleID            string          `json:"rule_id,omitempty"`
	Reason            string          `json:"reason"`
	ModifiedArguments json.RawMessage `json:"modified_arguments,omitempty"`
}

type auditMeta struct {
	CallID           string `json:"call_id,omitempty"`
	Tool             string `json:"tool,omitempty"`
	TargetCapability string `json:"target_capability,omitempty"`
	TargetMethod     string `json:"target_method,omitempty"`
	Decision         string `json:"decision"`
	RuleID           string `json:"rule_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ArgumentsDigest  string `json:"arguments_digest,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	ConfirmToken     string `json:"confirm_token,omitempty"`
	ExpiresAt        int64  `json:"expires_at,omitempty"`
	TS               int64  `json:"ts"`
}
