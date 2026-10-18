package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

var (
	toolNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	capabilityPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	targetMethodPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
)

type registry struct {
	mu    sync.RWMutex
	tools map[string]toolcontract.Definition
}

func newRegistry() *registry {
	return &registry{tools: make(map[string]toolcontract.Definition)}
}

func (r *registry) put(def toolcontract.Definition) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, found := r.tools[def.Name]
	if found && existing.SourcePlugin != def.SourcePlugin {
		return false, fmt.Errorf("tool %s is owned by %s: %w", def.Name, existing.SourcePlugin,
			pluginsdk.ErrCode("tool_name_conflict", "tool name is already registered"))
	}
	r.tools[def.Name] = def
	return found, nil
}

func (r *registry) remove(name, source string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	def, found := r.tools[name]
	if !found {
		return false, nil
	}
	if def.SourcePlugin != source {
		return false, pluginsdk.ErrCode("tool_owner_mismatch", "tool is owned by another plugin")
	}
	delete(r.tools, name)
	return true, nil
}

func (r *registry) get(name string) (toolcontract.Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.tools[name]
	return def, ok
}

func (r *registry) snapshot() []toolcontract.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]toolcontract.Definition, 0, len(r.tools))
	for _, def := range r.tools {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *registry) available(snapshot hostSnapshot) []toolcontract.Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]toolcontract.Definition, 0, len(r.tools))
	for _, def := range r.tools {
		if snapshot.available(def.SourcePlugin, def.TargetCapability, def.TargetMethod) {
			out = append(out, def)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func validateDefinition(in toolcontract.RegisterInput) (toolcontract.Definition, error) {
	if !validToolName(in.Name) {
		return toolcontract.Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "name must match ^[a-z][a-z0-9_]{0,63}$")
	}
	descriptionBytes := len([]byte(strings.TrimSpace(in.Description)))
	if descriptionBytes == 0 || descriptionBytes > 8192 {
		return toolcontract.Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "description must be 1 to 8192 bytes")
	}
	risk := toolcontract.NormalizeRisk(in.Risk)
	if risk == "" {
		return toolcontract.Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "risk must be low, medium or high")
	}
	if !capabilityPattern.MatchString(in.TargetCapability) {
		return toolcontract.Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "invalid target_capability")
	}
	if in.TargetCapability == "host" || in.TargetCapability == "tools" {
		return toolcontract.Definition{}, pluginsdk.ErrCode("tool_target_reserved", "target capability is reserved")
	}
	if !targetMethodPattern.MatchString(in.TargetMethod) {
		return toolcontract.Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "invalid target_method")
	}
	if err := validateSchema(in.InputSchema); err != nil {
		return toolcontract.Definition{}, err
	}
	return toolcontract.Definition{
		Name:             in.Name,
		Description:      in.Description,
		InputSchema:      in.InputSchema,
		Risk:             risk,
		TargetCapability: in.TargetCapability,
		TargetMethod:     in.TargetMethod,
	}, nil
}

func validateSchema(raw json.RawMessage) error {
	if len(raw) == 0 {
		return pluginsdk.ErrCode("tool_definition_invalid", "input_schema is required")
	}
	if len(raw) > 64<<10 {
		return pluginsdk.ErrCode("tool_definition_invalid", "input_schema exceeds 64 KiB")
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return pluginsdk.ErrCode("tool_definition_invalid", "input_schema must be a JSON object")
	}
	var schemaType string
	if err := json.Unmarshal(schema["type"], &schemaType); err != nil || schemaType != "object" {
		return pluginsdk.ErrCode("tool_definition_invalid", "input_schema.type must be object")
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &properties); err != nil || properties == nil {
		return pluginsdk.ErrCode("tool_definition_invalid", "input_schema.properties must be an object")
	}
	if rawRequired, ok := schema["required"]; ok {
		var required []string
		if err := json.Unmarshal(rawRequired, &required); err != nil {
			return pluginsdk.ErrCode("tool_definition_invalid", "input_schema.required must be a string array")
		}
		for _, name := range required {
			if _, exists := properties[name]; !exists {
				return pluginsdk.ErrCode("tool_definition_invalid", "required property is missing: "+name)
			}
		}
	}
	return nil
}

func validToolName(name string) bool {
	return toolNamePattern.MatchString(name)
}
