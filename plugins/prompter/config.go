package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
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

func loadConfig() (config, error) {
	data := defaultConfigJSON
	raw, err := os.ReadFile(configPath)
	switch {
	case err == nil:
		data = raw
	case errors.Is(err, fs.ErrNotExist):
	default:
		return config{}, err
	}

	return parseConfig(data)
}

func parseConfig(data []byte) (config, error) {
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

func normalizeMCP(servers []mcpEntry) error {
	seen := make(map[string]bool, len(servers))
	for i := range servers {
		servers[i].Name = strings.TrimSpace(servers[i].Name)
		servers[i].Description = strings.TrimSpace(servers[i].Description)
		if servers[i].Name == "" {
			return fmt.Errorf("prompt_config_invalid: mcp server name is required")
		}
		if seen[servers[i].Name] {
			return fmt.Errorf("prompt_config_invalid: duplicate mcp server %s", servers[i].Name)
		}
		seen[servers[i].Name] = true
		tools := servers[i].Tools[:0]
		for _, tool := range servers[i].Tools {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				tools = append(tools, tool)
			}
		}
		servers[i].Tools = tools
	}
	return nil
}
