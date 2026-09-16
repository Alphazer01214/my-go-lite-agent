// Command sandbox provides the policy Capability (ADR-0019).
//
// Capability: policy
//   - decide: {tool, arguments, workspace, sessionId} → {action, reason}
//
// Rules load from <exe>/config/permissions.json then override from
// <Workspace>/.liteagent/permissions.json (project layer wins; shallow).
package main

import (
	"encoding/json"
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
	Rules         []Rule `json:"rules"`
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

func decide(tool string, args json.RawMessage, workspace string) (string, string) {
	merged := RulesFile{DefaultAction: "allow"}
	if base, ok := loadRulesFile(filepath.Join(exeConfigDir(), "permissions.json")); ok {
		if base.DefaultAction != "" {
			merged.DefaultAction = base.DefaultAction
		}
		merged.Rules = base.Rules
	}
	if workspace != "" {
		if proj, ok := loadRulesFile(filepath.Join(workspace, ".liteagent", "permissions.json")); ok {
			if proj.DefaultAction != "" {
				merged.DefaultAction = proj.DefaultAction
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
			reason = "rule tool=" + r.Tool + " path=" + r.Path + " → " + r.Action
		}
	}
	if best != "" {
		return best, reason
	}
	return merged.DefaultAction, "defaultAction=" + merged.DefaultAction
}

func main() {
	s := pluginsdk.New()
	s.Handle("policy", "decide", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
			Workspace string          `json:"workspace"`
			SessionID string          `json:"sessionId"`
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
		action, reason := decide(in.Tool, in.Arguments, in.Workspace)
		return json.Marshal(map[string]any{
			"action": action,
			"reason": reason,
			"tool":   in.Tool,
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
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "defaultAction", "value": "allow", "type": "string"},
				{"name": "rulesPath", "value": filepath.Join(exeConfigDir(), "permissions.json"), "type": "string"},
			},
		})
	})
	s.Handle("config", "schema", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{
			"fields": []map[string]any{
				{"name": "defaultAction", "type": "string", "default": "allow",
					"description": "Fallback action when no rule matches"},
			},
		})
	})
	s.Handle("config", "reload", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"ok": true})
	})
	_ = s.Serve()
}
