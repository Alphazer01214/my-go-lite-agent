package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	maxSkills                = 256
	maxSkillDescriptionBytes = 1024
	maxSkillFileBytes        = 512 * 1024
	maxSkillContentBytes     = 256 * 1024
	skillCacheTTL            = 5 * time.Second

	skillSourceGlobal    = "global"
	skillSourceWorkspace = "workspace"
	skillSourceConfig    = "config"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

var skillReadParameters = []byte(`{
  "type": "object",
  "properties": {
    "name": {"type": "string", "description": "Skill name from the runtime context."}
  },
  "required": ["name"],
  "additionalProperties": false
}`)

type skillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
	Path        string `json:"path,omitempty"`
	Enabled     bool   `json:"enabled"`
	Error       string `json:"error,omitempty"`
}

type skillReadArgs struct {
	Name string `json:"name"`
}

func (p *plugin) skillCatalog(workspace string, force bool) ([]skillInfo, []string) {
	cfg := p.currentConfig()
	key := cfg.SkillsGlobalDir + "\x00" + cfg.SkillsWorkspaceDir + "\x00" + workspace

	p.skillMu.Lock()
	if !force && p.skillKey == key && time.Since(p.skillScannedAt) < skillCacheTTL {
		skills := append([]skillInfo(nil), p.skillSkills...)
		errors := append([]string(nil), p.skillErrors...)
		p.skillMu.Unlock()
		return skills, errors
	}
	p.skillMu.Unlock()

	skills, errors := scanSkills(cfg, workspace)

	p.skillMu.Lock()
	p.skillKey = key
	p.skillSkills = skills
	p.skillErrors = errors
	p.skillScannedAt = time.Now()
	p.skillMu.Unlock()
	return skills, errors
}

func (p *plugin) invalidateSkills() {
	p.skillMu.Lock()
	p.skillKey = ""
	p.skillScannedAt = time.Time{}
	p.skillMu.Unlock()
}

func (p *plugin) skillsReadable() bool {
	cfg := p.currentConfig()
	return cfg.SkillsGlobalDir != "" || cfg.SkillsWorkspaceDir != ""
}

func scanSkills(cfg config, workspace string) ([]skillInfo, []string) {
	skills := make([]skillInfo, 0)
	issues := make([]string, 0)
	byName := make(map[string]int)

	if cfg.SkillsGlobalDir != "" {
		scanSkillDir(cfg.SkillsGlobalDir, skillSourceGlobal, byName, &skills, &issues)
	}
	if cfg.SkillsWorkspaceDir != "" && strings.TrimSpace(workspace) != "" {
		dir := cfg.SkillsWorkspaceDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workspace, dir)
		}
		scanSkillDir(dir, skillSourceWorkspace, byName, &skills, &issues)
	}

	if cfg.SkillsGlobalDir == "" && cfg.SkillsWorkspaceDir == "" {
		for _, entry := range cfg.Skills {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				continue
			}
			if _, exists := byName[name]; exists {
				continue
			}
			if len(skills) >= maxSkills {
				issues = append(issues, fmt.Sprintf("skills truncated at %d", maxSkills))
				break
			}
			byName[name] = len(skills)
			skills = append(skills, skillInfo{
				Name:        name,
				Description: strings.TrimSpace(entry.Description),
				Source:      skillSourceConfig,
				Enabled:     true,
			})
		}
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, issues
}

func scanSkillDir(dir, source string, byName map[string]int, skills *[]skillInfo, issues *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			*issues = append(*issues, "skills dir not found: "+dir)
			return
		}
		*issues = append(*issues, "read skills dir "+dir+": "+err.Error())
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			*issues = append(*issues, fmt.Sprintf("read skill %s: %v", entry.Name(), err))
			continue
		}
		if len(data) > maxSkillFileBytes {
			*issues = append(*issues, fmt.Sprintf("skill %s exceeds %d bytes", entry.Name(), maxSkillFileBytes))
			continue
		}
		info, err := parseSkillFile(entry.Name(), data)
		if err != nil {
			*issues = append(*issues, fmt.Sprintf("skill %s: %v", entry.Name(), err))
			continue
		}
		info.Source = source
		info.Path = path
		info.Enabled = true

		if index, exists := byName[info.Name]; exists {
			existing := (*skills)[index]
			if existing.Source == source {
				*issues = append(*issues, fmt.Sprintf("duplicate skill %s in %s", info.Name, source))
				continue
			}
			if source == skillSourceWorkspace {
				(*skills)[index] = info
			}
			continue
		}
		if len(*skills) >= maxSkills {
			*issues = append(*issues, fmt.Sprintf("skills truncated at %d", maxSkills))
			return
		}
		byName[info.Name] = len(*skills)
		*skills = append(*skills, info)
	}
}

func parseSkillFile(dirName string, data []byte) (skillInfo, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	body := text
	name, description := "", ""

	if strings.HasPrefix(text, "---\n") {
		rest := text[4:]
		if index := strings.Index(rest, "\n---"); index >= 0 {
			front := rest[:index]
			body = strings.TrimLeft(rest[index+4:], "\n")
			for _, line := range strings.Split(front, "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "name":
					name = unquoteValue(value)
				case "description":
					description = unquoteValue(value)
				}
			}
		}
	}
	if name == "" {
		name = dirName
	}
	if !skillNamePattern.MatchString(name) {
		return skillInfo{}, fmt.Errorf("invalid skill name %q", name)
	}
	if description == "" {
		description = firstBodyLine(body)
	}
	description = strings.TrimSpace(truncateSkillText(description, maxSkillDescriptionBytes))
	return skillInfo{Name: name, Description: description}, nil
}

func firstBodyLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			line = strings.TrimSpace(strings.TrimLeft(line, "#"))
			if line == "" {
				continue
			}
		}
		return line
	}
	return ""
}

func unquoteValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func findSkill(skills []skillInfo, name string) (skillInfo, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return skillInfo{}, false
}

func (p *plugin) handleSkillInfo(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	cfg := p.currentConfig()
	tools := make([]providerTool, 0, 1)
	if cfg.SkillsGlobalDir != "" || cfg.SkillsWorkspaceDir != "" {
		tools = append(tools, providerTool{
			Name:        "skill_read",
			Description: "Load the full SKILL.md for a skill listed in the runtime context. Use the exact skill name from the Skills section.",
			Parameters:  skillReadParameters,
			Method:      "read",
		})
	}
	return json.Marshal(providerInfo{Capability: "skill", Tools: tools})
}

func (p *plugin) handleSkillRead(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args skillReadArgs
	if err := decodeProviderArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "name is required")
	}

	skills, _ := p.skillCatalog(input.Workspace, false)
	skill, ok := findSkill(skills, name)
	if !ok {
		skills, _ = p.skillCatalog(input.Workspace, true)
		skill, ok = findSkill(skills, name)
	}
	if !ok {
		return nil, errSkillNotFound(name)
	}
	if skill.Path == "" {
		return nil, errSkillUnavailable(name)
	}

	data, err := os.ReadFile(skill.Path)
	if err != nil {
		return nil, pluginsdk.ErrCode(codeSkillReadFailed, "read skill: "+err.Error())
	}
	content := string(data)
	truncated := false
	if len(content) > maxSkillContentBytes {
		content = truncateSkillText(content, maxSkillContentBytes)
		truncated = true
	}

	result := providerToolSuccess(content, map[string]any{
		"name":   skill.Name,
		"source": skill.Source,
		"path":   skill.Path,
	})
	result.Truncated = truncated
	return json.Marshal(result)
}

func skillEntries(skills []skillInfo) []skillEntry {
	if len(skills) == 0 {
		return nil
	}
	out := make([]skillEntry, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skillEntry{Name: skill.Name, Description: skill.Description})
	}
	return out
}

func truncateSkillText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
