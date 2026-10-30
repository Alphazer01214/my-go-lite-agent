package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"
)

type renderedSection struct {
	order int
	name  string
	text  string
}

func (c config) renderStatic(vars map[string]string) (string, error) {
	sections := make([]renderedSection, 0, len(c.PromptSections))
	for _, section := range c.PromptSections {
		if section.Track != trackStatic || section.Text == "" {
			continue
		}
		text, err := interpolate(section.Text, vars, false)
		if err != nil {
			return "", err
		}
		if text = strings.TrimSpace(text); text != "" {
			sections = append(sections, renderedSection{order: section.Order, name: section.Name, text: text})
		}
	}
	sortSections(sections)
	if len(sections) == 0 {
		return "", fmt.Errorf("prompt_render_failed: static track is empty")
	}
	return joinSections(sections), nil
}

func (c config) renderRuntime(vars map[string]string) (string, error) {
	sections := []renderedSection{{
		order: 0,
		name:  "environment",
		text:  renderEnvironment(vars),
	}}
	for _, section := range c.PromptSections {
		if section.Track != trackDynamic || section.Text == "" {
			continue
		}
		text, err := interpolate(section.Text, vars, true)
		if err != nil {
			return "", err
		}
		if text = strings.TrimSpace(text); text != "" {
			sections = append(sections, renderedSection{order: section.Order, name: section.Name, text: text})
		}
	}
	if text := renderSkills(c.Skills); text != "" {
		sections = append(sections, renderedSection{order: 200, name: "skills", text: text})
	}
	if text := renderMCPServers(c.MCPServers); text != "" {
		sections = append(sections, renderedSection{order: 250, name: "mcp", text: text})
	}
	sortSections(sections)
	body := joinSections(sections)
	if body == "" {
		return "", fmt.Errorf("prompt_render_failed: runtime context is empty")
	}
	return "<runtime_context>\n" +
		"Current runtime context. This snapshot supersedes earlier runtime-context snapshots.\n\n" +
		body + "\n</runtime_context>", nil
}

func renderEnvironment(vars map[string]string) string {
	lines := []string{"## Environment"}
	if workspace := strings.TrimSpace(vars["workspace"]); workspace != "" {
		lines = append(lines, "Workspace: "+workspace)
	}
	if model := strings.TrimSpace(vars["model"]); model != "" {
		lines = append(lines, "Model: "+model)
	}
	platform := strings.TrimSpace(vars["platform"])
	if platform == "" {
		platform = runtime.GOOS
	}
	lines = append(lines, "Platform: "+platform)
	date := strings.TrimSpace(vars["date"])
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	lines = append(lines, "Date: "+date)
	return strings.Join(lines, "\n")
}

func renderSkills(skills []skillEntry) string {
	if len(skills) == 0 {
		return ""
	}
	lines := []string{"## Skills"}
	for _, skill := range skills {
		line := "- " + skill.Name
		if skill.Description != "" {
			line += ": " + skill.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderMCPServers(servers []mcpEntry) string {
	if len(servers) == 0 {
		return ""
	}
	lines := []string{"## MCP Servers"}
	for _, server := range servers {
		line := "- " + server.Name
		if server.Description != "" {
			line += ": " + server.Description
		}
		if len(server.Tools) > 0 {
			line += " (tools: " + strings.Join(server.Tools, ", ") + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func interpolate(text string, vars map[string]string, allowDynamic bool) (string, error) {
	var firstErr error
	out := promptVariablePattern.ReplaceAllStringFunc(text, func(match string) string {
		if firstErr != nil {
			return match
		}
		name := match[2 : len(match)-2]
		if !allowDynamic {
			firstErr = fmt.Errorf("prompt_variable_not_allowed: %s", name)
			return match
		}
		value, ok := vars[name]
		if !ok {
			firstErr = fmt.Errorf("prompt_variable_unknown: %s", name)
			return match
		}
		return value
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func sortSections(sections []renderedSection) {
	sort.SliceStable(sections, func(i, j int) bool {
		if sections[i].order != sections[j].order {
			return sections[i].order < sections[j].order
		}
		return sections[i].name < sections[j].name
	})
}

func joinSections(sections []renderedSection) string {
	texts := make([]string, 0, len(sections))
	for _, section := range sections {
		if text := strings.TrimSpace(section.text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n\n"))
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func promptVars(workspace, model string) map[string]string {
	return map[string]string{
		"workspace": workspace,
		"model":     model,
		"platform":  runtime.GOOS,
		"date":      time.Now().Format("2006-01-02"),
	}
}
