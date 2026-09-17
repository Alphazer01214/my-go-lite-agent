// Command sandbox provides the policy Capability (ADR-0019).
//
// Capability: policy
//   - decide: {tool, arguments, workspace, sessionId} -> {action, reason}
//
// Rules load from <exe>/config/permissions.json then override from
// <Workspace>/.liteagent/permissions.json (project layer wins; shallow).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type Rule struct {
	Tool   string `json:"tool"`
	Path   string `json:"path"`
	Action string `json:"action"` // allow | ask | deny
}

type RulesFile struct {
	DefaultAction string `json:"defaultAction"`
	// SeverityPolicy maps tool severity (low|medium|high) → action when no
	// explicit rule matches. Plugin authors declare severity on tool schemas.
	SeverityPolicy map[string]string `json:"severityPolicy,omitempty"`
	Rules          []Rule            `json:"rules"`
}

func defaultSeverityPolicy() map[string]string {
	return map[string]string{
		"low":    "allow",
		"medium": "ask",
		"high":   "ask",
	}
}

func mergeSeverityPolicy(base map[string]string) map[string]string {
	out := defaultSeverityPolicy()
	for k, v := range base {
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.ToLower(strings.TrimSpace(v))
		switch k {
		case "low", "medium", "high":
			switch v {
			case "allow", "ask", "deny":
				out[k] = v
			}
		}
	}
	return out
}

func exeConfigDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "config"
	}
	return filepath.Join(filepath.Dir(exe), "config")
}

func loadRulesFile(path string) (RulesFile, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RulesFile{}, false
	}
	var f RulesFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return RulesFile{}, false
	}
	return f, true
}

func actionRank(a string) int {
	switch a {
	case "deny":
		return 3
	case "ask":
		return 2
	case "allow":
		return 1
	default:
		return 0
	}
}

func matchTool(ruleTool, tool string) bool {
	if ruleTool == "" || ruleTool == "*" {
		return true
	}
	return strings.EqualFold(ruleTool, tool)
}

func matchPath(pattern, path string) bool {
	if pattern == "" || pattern == "*" || pattern == "**" {
		return true
	}
	p := filepath.ToSlash(path)
	// Support simple ** prefix/suffix.
	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := strings.TrimSuffix(parts[0], "/")
		suffix := strings.TrimPrefix(parts[1], "/")
		if prefix != "" && !strings.HasPrefix(p, strings.TrimSuffix(prefix, "/")) {
			// also allow prefix as path element
			if !strings.Contains(p, prefix) {
				return false
			}
		}
		if suffix != "" && suffix != "*" {
			if ok, err := filepath.Match(suffix, filepath.Base(p)); err != nil || !ok {
				if !strings.HasSuffix(p, strings.TrimPrefix(suffix, "*")) {
					return false
				}
			}
		}
		return true
	}
	if ok, err := filepath.Match(filepath.ToSlash(pattern), p); err == nil && ok {
		return true
	}
	// Directory-prefix match: "src" matches "src/a/b".
	if strings.HasPrefix(p, strings.TrimSuffix(filepath.ToSlash(pattern), "/")+"/") {
		return true
	}
	return false
}

// extractPath pulls a path-like field from tool arguments for rule matching.
func extractPath(args json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return ""
	}
	for _, k := range []string{"path", "cwd", "file", "dir"} {
		if v, ok := m[k].(string); ok {
			return v
		}
	}
	return ""
}

func decide(tool string, args json.RawMessage, workspace, severity string) (string, string) {
	merged := RulesFile{DefaultAction: "allow"}
	if base, ok := loadRulesFile(filepath.Join(exeConfigDir(), "permissions.json")); ok {
		if base.DefaultAction != "" {
			merged.DefaultAction = base.DefaultAction
		}
		merged.SeverityPolicy = base.SeverityPolicy
		merged.Rules = base.Rules
	}
	if workspace != "" {
		if proj, ok := loadRulesFile(filepath.Join(workspace, ".liteagent", "permissions.json")); ok {
			if proj.DefaultAction != "" {
				merged.DefaultAction = proj.DefaultAction
			}
			if proj.SeverityPolicy != nil {
				merged.SeverityPolicy = proj.SeverityPolicy
			}
			// Project rules replace default list (shallow merge, spec).
			merged.Rules = proj.Rules
		}
	}
	path := extractPath(args)
	best := ""
	bestRank := 0
	reason := "no matching rule"
	for _, r := range merged.Rules {
		if !matchTool(r.Tool, tool) {
			continue
		}
		if !matchPath(r.Path, path) {
			continue
		}
		rank := actionRank(r.Action)
		if rank > bestRank {
			best = r.Action
			bestRank = rank
			reason = "rule tool=" + r.Tool + " path=" + r.Path + " -> " + r.Action
		}
	}
	if best != "" {
		return best, reason
	}
	// Severity map (plugin-declared tool risk) before bare defaultAction.
	if severity != "" {
		pol := mergeSeverityPolicy(merged.SeverityPolicy)
		if action, ok := pol[strings.ToLower(strings.TrimSpace(severity))]; ok {
			return action, "severity=" + severity + " -> " + action
		}
	}
	return merged.DefaultAction, "defaultAction=" + merged.DefaultAction
}

func globalRulesPath() string {
	return filepath.Join(exeConfigDir(), "permissions.json")
}

// confirmViaHost asks the Render Medium through Host agent.confirm.
// Returns true only when the medium approved. Missing host/hook → deny.
func confirmViaHost(s *pluginsdk.Server, tool string, args json.RawMessage, workspace, sessionID string) bool {
	payload, _ := json.Marshal(map[string]any{
		"tool":      tool,
		"arguments": args,
		"workspace": workspace,
		"sessionId": sessionID,
	})
	raw, err := s.CallTo("agent", "agent", "confirm", payload)
	if err != nil {
		return false
	}
	var out struct {
		Approved bool `json:"approved"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.Approved
}

func loadGlobalRules() RulesFile {
	f, ok := loadRulesFile(globalRulesPath())
	if !ok {
		return RulesFile{DefaultAction: "allow"}
	}
	if f.DefaultAction == "" {
		f.DefaultAction = "allow"
	}
	return f
}

func saveGlobalRules(f RulesFile) error {
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	dir := exeConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "permissions.json"), raw, 0o600)
}

func handleConfigCap(method string, payload json.RawMessage) (json.RawMessage, error) {
	switch method {
	case "get":
		cfg := loadGlobalRules()
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "defaultAction", "value": cfg.DefaultAction, "type": "string"},
				{"name": "severityPolicy", "value": mergeSeverityPolicy(cfg.SeverityPolicy), "type": "object"},
				{"name": "rulesPath", "value": globalRulesPath(), "type": "string", "readOnly": true},
			},
			"defaultAction":  cfg.DefaultAction,
			"severityPolicy": mergeSeverityPolicy(cfg.SeverityPolicy),
		})
	case "schema":
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "defaultAction", "type": "string", "default": "allow",
					"description": "Fallback action when no rule and no severity match (allow|ask|deny)"},
				{"name": "severityPolicy", "type": "object", "default": defaultSeverityPolicy(),
					"description": "Map tool severity (low|medium|high) → allow|ask|deny when no explicit rule matches"},
			},
		})
	case "set":
		var in map[string]any
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		cfg := loadGlobalRules()
		if raw, ok := in["defaultAction"]; ok {
			v := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
			switch v {
			case "allow", "ask", "deny":
				cfg.DefaultAction = v
			default:
				return nil, &protocol.FrameError{
					Code:    "bad_assignment",
					Message: "defaultAction must be allow|ask|deny",
				}
			}
		}
		if raw, ok := in["severityPolicy"]; ok {
			b, err := json.Marshal(raw)
			if err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
			var patch map[string]string
			if err := json.Unmarshal(b, &patch); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
			if cfg.SeverityPolicy == nil {
				cfg.SeverityPolicy = map[string]string{}
			}
			for k, v := range patch {
				k = strings.ToLower(strings.TrimSpace(k))
				v = strings.ToLower(strings.TrimSpace(v))
				switch k {
				case "low", "medium", "high":
					switch v {
					case "allow", "ask", "deny":
						cfg.SeverityPolicy[k] = v
					default:
						return nil, &protocol.FrameError{
							Code:    "bad_assignment",
							Message: "severityPolicy value must be allow|ask|deny",
						}
					}
				default:
					return nil, &protocol.FrameError{
						Code:    "bad_assignment",
						Message: "severityPolicy key must be low|medium|high",
					}
				}
			}
		}
		if err := saveGlobalRules(cfg); err != nil {
			return nil, &protocol.FrameError{Code: "save_failed", Message: err.Error()}
		}
		return json.Marshal(map[string]any{
			"ok":             true,
			"defaultAction":  cfg.DefaultAction,
			"severityPolicy": mergeSeverityPolicy(cfg.SeverityPolicy),
		})
	case "reload":
		cfg := loadGlobalRules()
		return json.Marshal(map[string]any{"ok": true, "defaultAction": cfg.DefaultAction})
	default:
		return nil, &protocol.FrameError{Code: "unknown_method", Message: "config." + method}
	}
}

func main() {
	s := pluginsdk.New()
	// policy.decide: resolve action; on ask, this plugin owns the Medium
	// round-trip via Host agent.confirm (sandbox → Host → CLI/Web), then
	// returns the final allow|deny. Agent must not re-confirm.
	s.Handle("policy", "decide", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Workspace string          `json:"workspace"`
			SessionID string          `json:"sessionId"`
			Severity  string          `json:"severity"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		if in.Tool == "" {
			return nil, &protocol.FrameError{Code: "bad_arguments", Message: "tool is required"}
		}
		if len(in.Arguments) == 0 {
			in.Arguments = json.RawMessage(`{}`)
		}
		action, reason := decide(in.Tool, in.Arguments, in.Workspace, in.Severity)
		if action == "ask" {
			approved := confirmViaHost(s, in.Tool, in.Arguments, in.Workspace, in.SessionID)
			if approved {
				action = "allow"
				reason = "user approved; " + reason
			} else {
				action = "deny"
				reason = "user denied; " + reason
			}
		}
		return json.Marshal(map[string]any{
			"action":   action,
			"reason":   reason,
			"tool":     in.Tool,
			"severity": in.Severity,
		})
	})
	s.Handle("policy", "rules", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Workspace string `json:"workspace"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		merged := RulesFile{DefaultAction: "allow"}
		if base, ok := loadRulesFile(filepath.Join(exeConfigDir(), "permissions.json")); ok {
			merged = base
			if merged.DefaultAction == "" {
				merged.DefaultAction = "allow"
			}
		}
		if in.Workspace != "" {
			if proj, ok := loadRulesFile(filepath.Join(in.Workspace, ".liteagent", "permissions.json")); ok {
				if proj.DefaultAction != "" {
					merged.DefaultAction = proj.DefaultAction
				}
				merged.Rules = proj.Rules
			}
		}
		return json.Marshal(merged)
	})
	s.Handle("config", "get", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("get", req.Payload)
	})
	s.Handle("config", "set", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("set", req.Payload)
	})
	s.Handle("config", "schema", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("schema", req.Payload)
	})
	s.Handle("config", "reload", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return handleConfigCap("reload", req.Payload)
	})
	_ = s.Serve()
}
