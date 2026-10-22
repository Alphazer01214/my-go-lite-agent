package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func validateDefinition(in RegisterInput) (Definition, error) {
	if !validToolName(in.Name) {
		return Definition{}, errToolDefinitionInvalid("name must match ^[a-z][a-z0-9_]{0,63}$")
	}
	descriptionBytes := len([]byte(strings.TrimSpace(in.Description)))
	if descriptionBytes == 0 || descriptionBytes > 8192 {
		return Definition{}, errToolDefinitionInvalid("description must be 1 to 8192 bytes")
	}
	if in.Risk < 0 || in.Risk > MaxRisk {
		return Definition{}, errToolDefinitionInvalid(fmt.Sprintf("risk must be between 0 and %d", MaxRisk))
	}
	if !capabilityPattern.MatchString(in.TargetCapability) {
		return Definition{}, errToolDefinitionInvalid("invalid target_capability")
	}
	if in.TargetCapability == "host" || in.TargetCapability == "tools" {
		return Definition{}, errToolTargetReserved("target capability is reserved")
	}
	if !targetMethodPattern.MatchString(in.TargetMethod) {
		return Definition{}, errToolDefinitionInvalid("invalid target_method")
	}
	tags, err := validateTags(in.Tags)
	if err != nil {
		return Definition{}, err
	}
	if err := validateSchema(in.InputSchema); err != nil {
		return Definition{}, err
	}
	return Definition{
		Name:             in.Name,
		Description:      in.Description,
		InputSchema:      in.InputSchema,
		Risk:             in.Risk,
		ParallelSafe:     in.ParallelSafe,
		Tags:             tags,
		TargetCapability: in.TargetCapability,
		TargetMethod:     in.TargetMethod,
	}, nil
}

func validateTags(tags []string) ([]string, error) {
	if len(tags) > 16 {
		return nil, errToolDefinitionInvalid("at most 16 tags are allowed")
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !tagPattern.MatchString(tag) {
			return nil, errToolDefinitionInvalid("invalid tag: " + tag)
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out, nil
}

func validateSchema(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errToolDefinitionInvalid("input_schema is required")
	}
	if len(raw) > 64<<10 {
		return errToolDefinitionInvalid("input_schema exceeds 64 KiB")
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return errToolDefinitionInvalid("input_schema must be a JSON object")
	}
	var schemaType string
	if err := json.Unmarshal(schema["type"], &schemaType); err != nil || schemaType != "object" {
		return errToolDefinitionInvalid("input_schema.type must be object")
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &properties); err != nil || properties == nil {
		return errToolDefinitionInvalid("input_schema.properties must be an object")
	}
	if rawRequired, ok := schema["required"]; ok {
		var required []string
		if err := json.Unmarshal(rawRequired, &required); err != nil {
			return errToolDefinitionInvalid("input_schema.required must be a string array")
		}
		for _, name := range required {
			if _, exists := properties[name]; !exists {
				return errToolDefinitionInvalid("required property is missing: " + name)
			}
		}
	}
	return nil
}

func validToolName(name string) bool {
	return toolNamePattern.MatchString(name)
}
