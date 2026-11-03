package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleSettingsGet(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	cfg := p.currentConfig()
	return json.Marshal(settingsGetResp{Values: settingsValues{
		DefaultAction:     cfg.DefaultAction,
		ConfirmTTLSeconds: cfg.ConfirmTTLSeconds,
		Rules:             rulesToSettings(cfg.Rules),
	}})
}

func (p *plugin) handleSettingsSave(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input settingsSaveReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}
	cfg := config{
		DefaultAction:     input.Values.DefaultAction,
		ConfirmTTLSeconds: input.Values.ConfirmTTLSeconds,
		Rules:             rulesFromSettings(input.Values.Rules),
	}
	if err := cfg.normalize(); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if err := saveConfig(p.configPath, cfg); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	p.setConfig(cfg)
	return json.Marshal(settingsSaveResp{Saved: true})
}

func rulesToSettings(rules []rule) []ruleSetting {
	out := make([]ruleSetting, 0, len(rules))
	for _, item := range rules {
		out = append(out, ruleSetting{
			RuleID:            item.RuleID,
			Action:            item.Action,
			Tool:              item.Scope.Tool,
			Capability:        item.Scope.Capability,
			Method:            item.Scope.Method,
			PathGlob:          item.Scope.PathGlob,
			CommandPattern:    item.Scope.CommandPattern,
			Priority:          item.Priority,
			Note:              item.Note,
			ModifiedArguments: compactJSON(item.ModifiedArguments),
		})
	}
	return out
}

func rulesFromSettings(items []ruleSetting) []rule {
	out := make([]rule, 0, len(items))
	for _, item := range items {
		if emptyRuleSetting(item) {
			continue
		}
		modified, _ := parseModifiedArguments(item.ModifiedArguments)
		out = append(out, rule{
			RuleID:   strings.TrimSpace(item.RuleID),
			Action:   strings.TrimSpace(item.Action),
			Priority: item.Priority,
			Note:     strings.TrimSpace(item.Note),
			Scope: ruleScope{
				Tool:           strings.TrimSpace(item.Tool),
				Capability:     strings.TrimSpace(item.Capability),
				Method:         strings.TrimSpace(item.Method),
				PathGlob:       strings.TrimSpace(item.PathGlob),
				CommandPattern: strings.TrimSpace(item.CommandPattern),
			},
			ModifiedArguments: modified,
		})
	}
	return out
}

func emptyRuleSetting(item ruleSetting) bool {
	return item.RuleID == "" && item.Action == "" && item.Tool == "" &&
		item.Capability == "" && item.Method == "" && item.PathGlob == "" &&
		item.CommandPattern == "" && item.Priority == 0 && item.Note == "" &&
		item.ModifiedArguments == ""
}

func parseModifiedArguments(raw string) (json.RawMessage, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
		return nil, pluginsdk.ErrCode(CodeRuleInvalid, "modified_arguments must be a JSON object")
	}
	return json.RawMessage(raw), nil
}

func compactJSON(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}
