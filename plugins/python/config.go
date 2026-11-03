package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const configPath = "config.json"

type config struct {
	Interpreter string `json:"interpreter"`
}

func loadConfig() config {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "python: load config: %v\n", err)
		}
		return config{}
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "python: invalid config: %v\n", err)
		return config{}
	}
	return cfg
}

// lookPathCommand only searches absolute PATH entries. On Windows,
// exec.LookPath also searches the current directory and rejects a match found
// there; the plugin's own python.exe would otherwise shadow the interpreter.
func lookPathCommand(name string) (string, error) {
	if filepath.IsAbs(name) {
		if isExecutableFile(name) {
			return name, nil
		}
		return "", os.ErrNotExist
	}

	extensions := []string{""}
	if runtime.GOOS == "windows" && filepath.Ext(name) == "" {
		extensions = filepath.SplitList(os.Getenv("PATHEXT"))
		if len(extensions) == 0 {
			extensions = []string{".COM", ".EXE", ".BAT", ".CMD"}
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		for _, extension := range extensions {
			candidate := filepath.Join(dir, name+extension)
			if isExecutableFile(candidate) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func resolveInterpreter(configured string, lookPath func(string) (string, error)) (string, bool) {
	if configured = strings.TrimSpace(configured); configured != "" {
		if resolved, err := lookPath(configured); err == nil {
			if resolved == "" {
				resolved = configured
			}
			return resolved, true
		}
		return configured, false
	}

	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3"}
	}
	for _, candidate := range candidates {
		if resolved, err := lookPath(candidate); err == nil {
			if resolved == "" {
				resolved = candidate
			}
			return resolved, true
		}
	}
	return candidates[0], false
}
