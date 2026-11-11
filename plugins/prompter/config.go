package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed config.example.json
var defaultConfigJSON []byte

var promptVariablePattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_]+)\}\}`)

var dynamicVariables = map[string]bool{
	"workspace": true,
	"model":     true,
	"platform":  true,
	"date":      true,
}

func loadConfig(path string) (config, error) {
	data := defaultConfigJSON
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		data = raw
	case errors.Is(err, fs.ErrNotExist):
	default:
		return config{}, err
	}

	return parseConfig(data)
}

func saveConfig(path string, cfg config) error {
	if err := cfg.normalize(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func parseConfig(data []byte) (config, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return config{}, fmt.Errorf("prompt_config_invalid: %w", err)
	}
	if _, ok := fields["mcp_servers"]; ok {
		return config{}, fmt.Errorf("prompt_config_invalid: mcp_servers is no longer supported, use mcpServers")
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config{}, fmt.Errorf("prompt_config_invalid: %w", err)
	}
	if err := cfg.normalize(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (c *config) normalize() error {
	if len(c.PromptSections) == 0 {
		return fmt.Errorf("prompt_config_invalid: prompt_sections must be non-empty")
	}

	seen := make(map[string]bool, len(c.PromptSections))
	hasStatic := false
	kept := c.PromptSections[:0]
	for i := range c.PromptSections {
		section := c.PromptSections[i]
		section.Name = strings.TrimSpace(section.Name)
		section.Track = strings.TrimSpace(section.Track)
		section.Text = strings.TrimSpace(section.Text)
		if section.Name == "" {
			return fmt.Errorf("prompt_config_invalid: section name is required")
		}
		if seen[section.Name] {
			return fmt.Errorf("prompt_section_duplicate: %s", section.Name)
		}
		seen[section.Name] = true
		if section.Track == "" {
			if section.Order < 0 {
				section.Track = trackStatic
			} else {
				section.Track = trackDynamic
			}
		}
		if section.Track != trackStatic && section.Track != trackDynamic {
			return fmt.Errorf("prompt_config_invalid: section %s has unknown track %q", section.Name, section.Track)
		}
		if section.Text != "" {
			if section.Track == trackStatic {
				if match := promptVariablePattern.FindStringSubmatch(section.Text); match != nil {
					return fmt.Errorf("prompt_variable_not_allowed: section %s uses {{%s}}", section.Name, match[1])
				}
				hasStatic = true
			} else if err := checkDynamicVariables(section.Name, section.Text); err != nil {
				return err
			}
			kept = append(kept, section)
		}
	}
	if !hasStatic {
		return fmt.Errorf("prompt_render_failed: static track is empty")
	}
	c.PromptSections = kept

	c.SkillsGlobalDir = strings.TrimSpace(c.SkillsGlobalDir)
	c.SkillsWorkspaceDir = strings.TrimSpace(c.SkillsWorkspaceDir)
	if c.MCPTimeoutMS == 0 {
		c.MCPTimeoutMS = defaultMCPTimeoutMS
	}
	if c.MCPTimeoutMS < 0 || c.MCPTimeoutMS > maxMCPTimeoutMS {
		return fmt.Errorf("prompt_config_invalid: mcp_timeout_ms must be between 1 and %d", maxMCPTimeoutMS)
	}

	if err := normalizeSkills(c.Skills); err != nil {
		return err
	}
	if err := normalizeMCP(c.MCPServers); err != nil {
		return err
	}
	return nil
}

func checkDynamicVariables(section, text string) error {
	for _, match := range promptVariablePattern.FindAllStringSubmatch(text, -1) {
		if !dynamicVariables[match[1]] {
			return fmt.Errorf("prompt_variable_unknown: section %s uses {{%s}}", section, match[1])
		}
	}
	return nil
}

func normalizeSkills(skills []skillEntry) error {
	seen := make(map[string]bool, len(skills))
	for i := range skills {
		skills[i].Name = strings.TrimSpace(skills[i].Name)
		skills[i].Description = strings.TrimSpace(skills[i].Description)
		if skills[i].Name == "" {
			return fmt.Errorf("prompt_config_invalid: skill name is required")
		}
		if seen[skills[i].Name] {
			return fmt.Errorf("prompt_config_invalid: duplicate skill %s", skills[i].Name)
		}
		seen[skills[i].Name] = true
	}
	return nil
}

// UnmarshalJSON 拒绝旧的 server 内字段：name 改由 mcpServers 键提供，transport 改为 type。
func (e *mcpEntry) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, ok := fields["name"]; ok {
		return fmt.Errorf("mcp server uses legacy field \"name\"; use the mcpServers key instead")
	}
	if _, ok := fields["transport"]; ok {
		return fmt.Errorf("mcp server uses legacy field \"transport\"; use \"type\" instead")
	}
	type plain mcpEntry
	var entry plain
	if err := json.Unmarshal(data, &entry); err != nil {
		return err
	}
	*e = mcpEntry(entry)
	return nil
}

func normalizeMCP(servers map[string]mcpEntry) error {
	for _, rawName := range sortedMCPNames(servers) {
		entry := servers[rawName]
		name := strings.TrimSpace(rawName)
		if name == "" {
			return fmt.Errorf("prompt_config_invalid: mcp server name is required")
		}
		if name != rawName {
			delete(servers, rawName)
			if _, exists := servers[name]; exists {
				return fmt.Errorf("prompt_config_invalid: duplicate mcp server %s", name)
			}
		}
		entry.Name = name
		entry.Description = strings.TrimSpace(entry.Description)
		entry.Type = strings.TrimSpace(entry.Type)
		entry.Command = strings.TrimSpace(entry.Command)
		entry.URL = strings.TrimSpace(entry.URL)
		entry.Cwd = strings.TrimSpace(entry.Cwd)
		if entry.Type == "" {
			if entry.URL != "" {
				entry.Type = mcpTransportHTTP
			} else {
				entry.Type = mcpTransportStdio
			}
		}
		switch entry.Type {
		case mcpTransportStdio, mcpTransportHTTP, mcpTransportSSE:
		default:
			return fmt.Errorf("prompt_config_invalid: mcp server %s has unknown type %q", name, entry.Type)
		}
		if entry.Enabled == nil {
			enabled := true
			entry.Enabled = &enabled
		}
		if entry.TimeoutMS < 0 || entry.TimeoutMS > maxMCPTimeoutMS {
			return fmt.Errorf("prompt_config_invalid: mcp server %s timeout_ms is out of range", name)
		}
		if strings.ContainsRune(entry.Command, 0) || strings.ContainsRune(entry.Cwd, 0) || strings.ContainsRune(entry.URL, 0) {
			return fmt.Errorf("prompt_config_invalid: mcp server %s has NUL in command, cwd, or url", name)
		}
		for _, arg := range entry.Args {
			if strings.ContainsRune(arg, 0) {
				return fmt.Errorf("prompt_config_invalid: mcp server %s has NUL in args", name)
			}
		}
		for key, value := range entry.Env {
			if !validMCPEnvName(key) || strings.ContainsRune(value, 0) {
				return fmt.Errorf("prompt_config_invalid: mcp server %s has invalid env %q", name, key)
			}
		}
		for key, value := range entry.Headers {
			if strings.TrimSpace(key) == "" || strings.ContainsRune(key, 0) || strings.ContainsRune(value, 0) {
				return fmt.Errorf("prompt_config_invalid: mcp server %s has invalid header %q", name, key)
			}
		}
		tools := entry.Tools[:0]
		for _, tool := range entry.Tools {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				tools = append(tools, tool)
			}
		}
		entry.Tools = tools
		servers[name] = entry
	}
	return nil
}

func sortedMCPNames(servers map[string]mcpEntry) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mcpGloballyEnabled(cfg config) bool {
	return cfg.MCPEnabled == nil || *cfg.MCPEnabled
}

func validMCPEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' {
			continue
		}
		if index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}
