package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const configFile = "config.json"

// config is the plugin configuration. It lives in the plugin working
// directory, which is the plugin directory: the Host starts a plugin with its
// own directory as the working directory.
type config struct {
	DefaultAction      string `json:"default_action"`
	RiskBands          []band `json:"risk_bands"`
	RulesFile          string `json:"rules_file"`
	AuditRetentionDays int    `json:"audit_retention_days"`
}

// band maps a risk ceiling to an action. The table is read from the lowest
// ceiling up: the first band whose max_risk covers the risk wins.
type band struct {
	MaxRisk int    `json:"max_risk"`
	Action  string `json:"action"`
}

// defaultConfig is the shipped policy.
//
// The risk scale is the one the tools plugin uses: 0 to 100. The band table
// below is the target mapping of this build: read only calls are allowed,
// anything that writes or executes is denied because the confirmation state
// does not exist yet. When the confirmation flow lands, the two deny bands
// become confirm bands; no code changes with them.
func defaultConfig() config {
	return config{
		DefaultAction: ActionAllow,
		RiskBands: []band{
			{MaxRisk: 49, Action: ActionAllow},
			{MaxRisk: 79, Action: ActionDeny},
			{MaxRisk: 100, Action: ActionDeny},
		},
		RulesFile:          "rules.json",
		AuditRetentionDays: 30,
	}
}

// loadConfig reads a configuration file. A missing file, a broken file or an
// invalid field falls back to the default value of that field.
func loadConfig(path string) config {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	var raw struct {
		DefaultAction      string `json:"default_action"`
		RiskBands          []band `json:"risk_bands"`
		RulesFile          string `json:"rules_file"`
		AuditRetentionDays int    `json:"audit_retention_days"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: config: %v\n", err)
		return cfg
	}
	if validAction(raw.DefaultAction) {
		cfg.DefaultAction = raw.DefaultAction
	}
	if bands, ok := validBands(raw.RiskBands); ok {
		cfg.RiskBands = bands
	} else if len(raw.RiskBands) > 0 {
		fmt.Fprintln(os.Stderr, "sandbox: config: invalid risk_bands, using defaults")
	}
	if name := validRulesFile(raw.RulesFile); name != "" {
		cfg.RulesFile = name
	}
	if raw.AuditRetentionDays > 0 && raw.AuditRetentionDays <= 3650 {
		cfg.AuditRetentionDays = raw.AuditRetentionDays
	}
	return cfg
}

// validBands checks a band table and returns it sorted by ceiling.
func validBands(bands []band) ([]band, bool) {
	if len(bands) == 0 {
		return nil, false
	}
	out := make([]band, 0, len(bands))
	for _, item := range bands {
		if item.MaxRisk < 0 || item.MaxRisk > 100 || !validAction(item.Action) {
			return nil, false
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MaxRisk < out[j].MaxRisk })
	return out, true
}

// validRulesFile keeps the rule file inside the plugin directory.
func validRulesFile(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return ""
	}
	return name
}

// bandAction returns the action of the band that covers a risk value. An
// empty result means no band covers it and the default action applies.
func (c config) bandAction(risk int) (string, bool) {
	for _, item := range c.RiskBands {
		if risk <= item.MaxRisk {
			return item.Action, true
		}
	}
	return "", false
}

// bandRange returns the human readable range of the band that covers a risk.
func (c config) bandRange(risk int) (int, int, bool) {
	previous := -1
	for _, item := range c.RiskBands {
		if risk <= item.MaxRisk {
			return previous + 1, item.MaxRisk, true
		}
		previous = item.MaxRisk
	}
	return 0, 0, false
}

// save writes the configuration back to a file.
func (c config) save(path string) error {
	body, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, body)
}

// atomicWriteFile writes through a temporary file and a rename, so a reader
// never sees a half written file.
func atomicWriteFile(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, 0o644); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
