package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	toolNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	methodPattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

func reservedCapability(capability string) bool {
	switch capability {
	case "host", "tool", "tools":
		return true
	default:
		return false
	}
}

func validateProviderInfo(capability string, raw json.RawMessage) ([]catalogTool, *diagnostic) {
	if len(raw) == 0 {
		return nil, &diagnostic{
			Capability: capability,
			Code:       "info_invalid",
			Message:    "empty info response",
		}
	}
	if len(raw) > maxProviderInfoBytes {
		return nil, &diagnostic{
			Capability: capability,
			Code:       "info_invalid",
			Message:    fmt.Sprintf("info response exceeds %d bytes", maxProviderInfoBytes),
		}
	}

	var info providerInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, &diagnostic{
			Capability: capability,
			Code:       "info_invalid",
			Message:    err.Error(),
		}
	}
	if info.Capability != capability {
		return nil, &diagnostic{
			Capability: capability,
			Code:       "capability_mismatch",
			Message:    fmt.Sprintf("info reports capability %q, configured as %q", info.Capability, capability),
		}
	}
	if len(info.Tools) > maxTools {
		return nil, &diagnostic{
			Capability: capability,
			Code:       "definition_invalid",
			Message:    fmt.Sprintf("capability exposes too many tools: %d > %d", len(info.Tools), maxTools),
		}
	}

	seen := make(map[string]bool, len(info.Tools))
	tools := make([]catalogTool, 0, len(info.Tools))
	for _, item := range info.Tools {
		tool, err := validateToolDefinition(capability, item)
		if err != nil {
			return nil, &diagnostic{
				Capability: capability,
				Code:       "definition_invalid",
				Message:    fmt.Sprintf("tool %q: %v", item.Name, err),
			}
		}
		if seen[tool.Name] {
			return nil, &diagnostic{
				Capability: capability,
				Code:       "definition_invalid",
				Message:    fmt.Sprintf("duplicate tool name %q", tool.Name),
			}
		}
		seen[tool.Name] = true
		tools = append(tools, tool)
	}
	return tools, nil
}

func validateToolDefinition(capability string, item providerTool) (catalogTool, error) {
	if !toolNamePattern.MatchString(item.Name) {
		return catalogTool{}, errors.New("name must match ^[a-z][a-z0-9_]{0,63}$")
	}
	if len(item.Description) == 0 || len(item.Description) > maxDescriptionBytes {
		return catalogTool{}, fmt.Errorf("description must be between 1 and %d bytes", maxDescriptionBytes)
	}
	if !methodPattern.MatchString(item.Method) {
		return catalogTool{}, errors.New("method must match ^[a-z][a-z0-9_]{0,63}$")
	}
	if item.Method == "info" {
		return catalogTool{}, errors.New("method info is reserved for discovery")
	}
	if err := validateParameters(item.Parameters); err != nil {
		return catalogTool{}, err
	}
	return catalogTool{
		Name:        item.Name,
		Description: item.Description,
		Parameters:  append(json.RawMessage(nil), item.Parameters...),
		Capability:  capability,
		Method:      item.Method,
	}, nil
}

func validateParameters(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("parameters is required")
	}
	if len(raw) > maxParametersBytes {
		return fmt.Errorf("parameters exceeds %d bytes", maxParametersBytes)
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return errors.New("parameters must be a JSON object")
	}
	var typ string
	if err := json.Unmarshal(schema["type"], &typ); err != nil || typ != "object" {
		return errors.New("parameters.type must be object")
	}
	return nil
}

func normalizeObject(raw json.RawMessage, field string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", field)
	}
	return raw, nil
}

func validateToolResult(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("empty tool result")
	}
	var result struct {
		Content   *string         `json:"content"`
		IsError   *bool           `json:"is_error"`
		Truncated *bool           `json:"truncated"`
		Meta      json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("invalid tool result: %w", err)
	}
	if result.Content == nil {
		return errors.New("content is required")
	}
	if len(*result.Content) > maxContentBytes {
		return fmt.Errorf("content exceeds %d bytes", maxContentBytes)
	}
	meta := bytes.TrimSpace(result.Meta)
	if len(meta) > 0 && !bytes.Equal(meta, []byte("null")) {
		if len(meta) > maxMetaBytes {
			return fmt.Errorf("meta exceeds %d bytes", maxMetaBytes)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(meta, &object); err != nil || object == nil {
			return errors.New("meta must be a JSON object")
		}
	}
	return nil
}

func conflictMessage(name string, tools []catalogTool) string {
	sources := make([]string, 0, len(tools))
	for _, tool := range tools {
		sources = append(sources, tool.Capability)
	}
	return fmt.Sprintf("tool %q is provided by multiple capabilities: %s", name, strings.Join(sources, ", "))
}
