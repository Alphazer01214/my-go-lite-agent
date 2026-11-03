package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	configFile        = "config.json"
	defaultAction     = ActionAllow
	defaultConfirmTTL = 300
	minConfirmTTL     = 30
	maxConfirmTTL     = 3600
)

type config struct {
	DefaultAction     string `json:"default_action"`
	ConfirmTTLSeconds int    `json:"confirm_ttl_seconds"`
	Rules             []rule `json:"rules"`
}

func defaultConfig() config {
	return config{
		DefaultAction:     defaultAction,
		ConfirmTTLSeconds: defaultConfirmTTL,
		Rules:             []rule{},
	}
}

func loadConfig(path string) (config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var raw config
	if err := json.Unmarshal(data, &raw); err != nil {
		return defaultConfig(), err
	}
	if err := raw.normalize(); err != nil {
		return defaultConfig(), err
	}
	return raw, nil
}

func (c *config) normalize() error {
	if strings.TrimSpace(c.DefaultAction) == "" {
		c.DefaultAction = defaultAction
	}
	c.DefaultAction = strings.TrimSpace(c.DefaultAction)
	if !validOutcomeAction(c.DefaultAction) {
		return fmt.Errorf("default_action must be allow, confirm or deny")
	}
	if c.ConfirmTTLSeconds == 0 {
		c.ConfirmTTLSeconds = defaultConfirmTTL
	}
	if c.ConfirmTTLSeconds < minConfirmTTL || c.ConfirmTTLSeconds > maxConfirmTTL {
		return fmt.Errorf("confirm_ttl_seconds must be between %d and %d", minConfirmTTL, maxConfirmTTL)
	}

	seen := make(map[string]bool, len(c.Rules))
	for index := range c.Rules {
		c.Rules[index].RuleID = strings.TrimSpace(c.Rules[index].RuleID)
		if c.Rules[index].RuleID == "" {
			continue
		}
		if seen[c.Rules[index].RuleID] {
			return fmt.Errorf("duplicate rule_id %q", c.Rules[index].RuleID)
		}
		seen[c.Rules[index].RuleID] = true
	}

	now := time.Now().UnixMilli()
	seq := 0
	for index := range c.Rules {
		item := &c.Rules[index]
		if item.RuleID == "" {
			for {
				seq++
				candidate := fmt.Sprintf("r-%d", seq)
				if !seen[candidate] {
					item.RuleID = candidate
					seen[candidate] = true
					break
				}
			}
		}
		item.Action = strings.TrimSpace(item.Action)
		if item.Action == "" {
			item.Action = ActionConfirm
		}
		if !validAction(item.Action) {
			return fmt.Errorf("rule %s: unknown action %q", item.RuleID, item.Action)
		}
		item.Scope.Tool = strings.TrimSpace(item.Scope.Tool)
		item.Scope.Capability = strings.TrimSpace(item.Scope.Capability)
		item.Scope.Method = strings.TrimSpace(item.Scope.Method)
		item.Scope.PathGlob = strings.TrimSpace(item.Scope.PathGlob)
		item.Scope.CommandPattern = strings.TrimSpace(item.Scope.CommandPattern)
		if item.Scope.empty() {
			return fmt.Errorf("rule %s: scope must name at least one condition", item.RuleID)
		}
		if item.Action == ActionModify {
			modified, err := normalizeJSONObject(item.ModifiedArguments)
			if err != nil || len(modified) == 0 {
				return fmt.Errorf("rule %s: modified_arguments must be a JSON object for modify", item.RuleID)
			}
			item.ModifiedArguments = modified
		}
		if item.CreatedTS == 0 {
			item.CreatedTS = now
		}
	}
	return nil
}

func normalizeJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil || object == nil {
		return nil, fmt.Errorf("must be a JSON object")
	}
	return trimmed, nil
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
