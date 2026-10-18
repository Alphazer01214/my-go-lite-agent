package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type policyFile struct {
	Defaults map[string]string `json:"defaults"`
	Rules    []policyRule      `json:"rules"`
}

type policyRule struct {
	Capability      string `json:"capability,omitempty"`
	Method          string `json:"method,omitempty"`
	Tool            string `json:"tool,omitempty"`
	Action          string `json:"action"`
	WorkspacePrefix string `json:"workspace_prefix,omitempty"`
}

type policyStore struct {
	mu      sync.Mutex
	path    string
	modTime time.Time
	loaded  policyFile
}

type ruleContext struct {
	Capability string
	Method     string
	Tool       string
	Risk       string
	Workspace  string
	Source     string
}

func newPolicyStore(path string) *policyStore {
	store := &policyStore{path: path, loaded: defaultPolicy()}
	store.reload()
	return store
}

func defaultPolicy() policyFile {
	return policyFile{
		Defaults: map[string]string{
			"low":    "allow",
			"medium": "allow",
			"high":   "deny",
		},
	}
}

func (s *policyStore) reload() {
	info, err := os.Stat(s.path)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.modTime.IsZero() && !info.ModTime().After(s.modTime) {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	policy := defaultPolicy()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return
	}
	for risk, action := range policy.Defaults {
		if action != "allow" && action != "deny" {
			policy.Defaults[risk] = defaultPolicy().Defaults[risk]
		}
	}
	s.loaded = policy
	s.modTime = info.ModTime()
}

func (s *policyStore) decide(ctx ruleContext) decision {
	s.reload()
	s.mu.Lock()
	policy := s.loaded
	s.mu.Unlock()
	for _, rule := range policy.Rules {
		if !ruleMatches(rule, ctx) {
			continue
		}
		return decision{
			Allowed: rule.Action == "allow", Risk: ctx.Risk,
			Reason:     policyReason(rule.Action, ctx),
			Capability: ctx.Capability, Method: ctx.Method, Tool: ctx.Tool,
		}
	}
	action := policy.Defaults[ctx.Risk]
	if action == "" {
		action = "deny"
	}
	return decision{
		Allowed: action == "allow", Risk: ctx.Risk,
		Reason:     policyReason(action, ctx),
		Capability: ctx.Capability, Method: ctx.Method, Tool: ctx.Tool,
	}
}

func ruleMatches(rule policyRule, ctx ruleContext) bool {
	if rule.Capability != "" && rule.Capability != ctx.Capability {
		return false
	}
	if rule.Method != "" && rule.Method != ctx.Method {
		return false
	}
	if rule.Tool != "" && rule.Tool != ctx.Tool {
		return false
	}
	if rule.WorkspacePrefix != "" && !workspaceWithin(ctx.Workspace, rule.WorkspacePrefix) {
		return false
	}
	return true
}

func workspaceWithin(workspace, prefix string) bool {
	if workspace == "" || prefix == "" {
		return workspace == prefix
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	prefixAbs, err := filepath.Abs(prefix)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(prefixAbs, workspaceAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func policyReason(action string, ctx ruleContext) string {
	verb := "denied"
	if action == "allow" {
		verb = "allowed"
	}
	return "sandbox " + verb + " " + ctx.Risk + " risk target " + ctx.Capability + "." + ctx.Method
}

type listOut struct {
	Tools []toolcontract.Definition `json:"tools"`
}
