package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configPath      = "config.json"
	defaultMaxDepth = 1
	maxAllowedDepth = 3
)

type config struct {
	MaxDepth int `json:"max_depth"`
}

func defaultConfig() config {
	return config{MaxDepth: defaultMaxDepth}
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
	var raw config
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, err
	}
	if raw.MaxDepth == 0 {
		return cfg, nil
	}
	if err := raw.validate(); err != nil {
		return cfg, err
	}
	return raw, nil
}

func saveConfig(path string, cfg config) error {
	if err := cfg.validate(); err != nil {
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

func (c config) validate() error {
	if c.MaxDepth < 1 || c.MaxDepth > maxAllowedDepth {
		return fmt.Errorf("max_depth must be between 1 and %d", maxAllowedDepth)
	}
	return nil
}
