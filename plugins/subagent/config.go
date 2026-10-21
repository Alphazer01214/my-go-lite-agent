package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const configFile = "config.json"

type config struct {
	MaxDepth        int `json:"max_depth"`
	DefaultMaxSteps int `json:"default_max_steps"`
}

func defaultConfig() config {
	return config{MaxDepth: 1, DefaultMaxSteps: 24}
}

func loadConfig() config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFile)
	if err != nil {
		return cfg
	}
	var raw config
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "subagent: read %s: %v\n", configFile, err)
		return cfg
	}
	if raw.MaxDepth >= 1 && raw.MaxDepth <= 3 {
		cfg.MaxDepth = raw.MaxDepth
	}
	if raw.DefaultMaxSteps > 0 {
		cfg.DefaultMaxSteps = raw.DefaultMaxSteps
	}
	return cfg
}
