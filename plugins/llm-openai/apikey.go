package main

import (
	"strings"
)

func resolveAPIKey(in *string, id string, old []modelConfig) string {
	if in == nil {
		return apiKeyFor(old, id)
	}
	v := strings.TrimSpace(*in)
	if v == "" {
		return ""
	}
	if !strings.Contains(v, "***") {
		return v
	}
	for _, model := range old {
		if model.APIKey != "" && maskKey(model.APIKey) == v {
			return model.APIKey
		}
	}
	return ""
}

func apiKeyFor(models []modelConfig, id string) string {
	if entry := findModel(models, id); entry != nil {
		return entry.APIKey
	}
	return ""
}

func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "***"
	}
	return k[:3] + "***" + k[len(k)-3:]
}

func maskedModels(models []modelConfig) []modelConfig {
	out := make([]modelConfig, 0, len(models))
	for _, model := range models {
		model.APIKey = maskKey(model.APIKey)
		out = append(out, model)
	}
	return out
}
