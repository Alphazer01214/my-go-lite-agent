package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// config.example.json is both the customization template and the shipped
// default. config.json remains the user override and is gitignored.
//
//go:embed config.example.json
var shippedConfig []byte

func defaultPromptSections() []promptSection {
	return []promptSection{
		{Name: "identity", Order: -1000, Text: "You are an AI agent powered by LiteAgent."},
		{Name: "persona", Order: 0, Text: "You are a coding agent. Your working directory is {{workspace}}."},
		{Name: "policy", Order: 500, Text: defaultSystemPrompt},
	}
}

func defaultConfig() config {
	return config{
		PromptSections:      defaultPromptSections(),
		MaxContextTokens:    defaultMaxContextTokens,
		CompactThreshold:    defaultCompactThreshold,
		RetainRecentTokens:  defaultRetainTokens,
		CompactMaxChars:     defaultCompactMaxChars,
		AgentsMDMaxBytes:    defaultAgentsMDMaxBytes,
		SnapshotMemoryLimit: defaultSnapshotLimit,
	}
}

func loadConfig() config {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "model-context: read %s: %v\n", configFile, err)
		}
		return shippedOrDefault()
	}
	var raw config
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "model-context: read %s: %v\n", configFile, err)
		return shippedOrDefault()
	}
	return normalizeConfig(raw)
}

// A missing or broken config.json must not silently fall back to the minimal
// built-in prompt; the embedded example is the shipped default.
func shippedOrDefault() config {
	var raw config
	if err := json.Unmarshal(shippedConfig, &raw); err != nil {
		return defaultConfig()
	}
	return normalizeConfig(raw)
}

func normalizeConfig(raw config) config {
	cfg := defaultConfig()
	if len(raw.PromptSections) > 0 {
		cfg.PromptSections = raw.PromptSections
	} else if strings.TrimSpace(raw.SystemPrompt) != "" {
		cfg.PromptSections = []promptSection{{
			Name: "system_prompt", Order: 0, Text: raw.SystemPrompt,
		}}
	}
	if len(raw.SubagentSections) > 0 {
		cfg.SubagentSections = raw.SubagentSections
	}
	if raw.MaxContextTokens != 0 {
		if raw.MaxContextTokens < 0 {
			cfg.MaxContextTokens = 0
		} else {
			cfg.MaxContextTokens = raw.MaxContextTokens
		}
	}
	if raw.CompactThreshold > 0 && raw.CompactThreshold <= 1 {
		cfg.CompactThreshold = raw.CompactThreshold
	}
	if raw.RetainRecentTokens > 0 {
		cfg.RetainRecentTokens = raw.RetainRecentTokens
	}
	if raw.CompactMaxChars > 0 {
		cfg.CompactMaxChars = raw.CompactMaxChars
	}
	if raw.AgentsMDMaxBytes > 0 {
		cfg.AgentsMDMaxBytes = raw.AgentsMDMaxBytes
	}
	if raw.SnapshotMemoryLimit > 0 {
		cfg.SnapshotMemoryLimit = raw.SnapshotMemoryLimit
	}
	return cfg
}
