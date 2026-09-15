// Command skill-manager discovers Workspace skills and injects them.
//
// Capability: skills
//   - list {workspace}
//   - get  {workspace, name}
//   - expand {workspace, text} → {text, injected:[names]}  // $skill tokens
//
// Capability: tools
//   - load_skill {name}  (uses workspace from call payload)
//
// Layout: <Workspace>/.liteagent/skills/<name>/SKILL.md
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const maxSkillBytes = 24 * 1024

var skillTriggerRe = regexp.MustCompile(`\$([A-Za-z0-9][A-Za-z0-9_-]*)`)

var toolSchemas = []map[string]any{
	{
		"name":        "load_skill",
		"description": "Load a Workspace skill by name and return its full text.",
		"readOnly":    true,
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]string{"type": "string", "description": "Skill name"},
			},
			"required": []string{"name"},
		},
	},
}

func skillsRoot(workspace string) string {
	if workspace == "" {
		return ""
	}
	return filepath.Join(workspace, ".liteagent", "skills")
}

func readSkill(workspace, name string) (string, error) {
	root := skillsRoot(workspace)
	if root == "" {
		return "", &protocol.FrameError{Code: "no_workspace", Message: "workspace required"}
	}
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: "invalid skill name"}
	}
	p := filepath.Join(root, name, "SKILL.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", &protocol.FrameError{Code: "skill_not_found", Message: "skill not found: " + name}
	}
	text := string(raw)
	if len(text) > maxSkillBytes {
		text = text[:maxSkillBytes] + "\n... (truncated)\n"
	}
	return text, nil
}

func listSkills(workspace string) []map[string]string {
	root := skillsRoot(workspace)
	out := []map[string]string{}
	if root == "" {
		return out
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), "SKILL.md")
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		summary := ""
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				summary = line
				if len(summary) > 120 {
					summary = summary[:120] + "…"
				}
				break
			}
		}
		out = append(out, map[string]string{"name": e.Name(), "summary": summary})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"] < out[j]["name"] })
	return out
}

// expandSkills replaces $name tokens with skill bodies (Skill Trigger).
func expandSkills(workspace, text string) (string, []string, []string) {
	var injected, missing []string
	out := skillTriggerRe.ReplaceAllStringFunc(text, func(tok string) string {
		name := tok[1:]
		body, err := readSkill(workspace, name)
		if err != nil {
			missing = append(missing, name)
			return tok
		}
		injected = append(injected, name)
		return fmt.Sprintf("\n[skill:%s]\n%s\n[/skill:%s]\n", name, strings.TrimSpace(body), name)
	})
	return out, injected, missing
}

func main() {
	s := pluginsdk.New()

	s.Handle("tools", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"tools": toolSchemas})
	})
	s.Handle("tools", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Workspace string          `json:"workspace"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if in.Name != "load_skill" {
			return nil, &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(in.Arguments, &args); err != nil {
			return nil, &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
		}
		body, err := readSkill(in.Workspace, args.Name)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"content": body})
	})

	s.Handle("skills", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Workspace string `json:"workspace"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		return json.Marshal(map[string]any{"skills": listSkills(in.Workspace)})
	})
	s.Handle("skills", "get", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Workspace string `json:"workspace"`
			Name      string `json:"name"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		body, err := readSkill(in.Workspace, in.Name)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"name": in.Name, "text": body})
	})
	s.Handle("skills", "expand", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Workspace string `json:"workspace"`
			Text      string `json:"text"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		out, injected, missing := expandSkills(in.Workspace, in.Text)
		return json.Marshal(map[string]any{
			"text":     out,
			"injected": injected,
			"missing":  missing,
		})
	})

	// Register skill catalog into Context Manager when possible (best-effort at start is not enough —
	// Agent refreshes via register after expand/list; also expose a refresh method).
	s.Handle("skills", "refreshCatalog", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Workspace string `json:"workspace"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		skills := listSkills(in.Workspace)
		var b strings.Builder
		b.WriteString("Available skills (trigger with $name or load_skill):\n")
		for _, sk := range skills {
			fmt.Fprintf(&b, "- $%s: %s\n", sk["name"], sk["summary"])
		}
		if len(skills) == 0 {
			b.WriteString("(none)\n")
		}
		payload, err := json.Marshal(map[string]any{"name": "skills", "order": 40, "text": b.String()})
		if err != nil {
			return nil, err
		}
		// Prefer context.registerSkill (CM skills table); fall back to segment.
		if _, err := s.Call("context", "registerSkill", payload); err != nil {
			if _, err2 := s.Call("system-prompt", "registerSegment", payload); err2 != nil {
				return json.Marshal(map[string]any{"ok": false, "error": err.Error()})
			}
		}
		return json.Marshal(map[string]any{"ok": true, "count": len(skills)})
	})

	_ = s.Serve()
}
