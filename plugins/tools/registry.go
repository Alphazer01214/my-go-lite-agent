package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const registryFile = "registry.json"

var (
	toolNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	tagPattern          = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	capabilityPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	targetMethodPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
)

type toolEntry struct {
	definition Definition
	generation int
}

type persistedRegistry struct {
	Version int             `json:"version"`
	Tools   []persistedTool `json:"tools"`
}

type persistedTool struct {
	Definition Definition `json:"definition"`
	Generation int        `json:"generation"`
}

type registry struct {
	mu    sync.RWMutex
	tools map[string]toolEntry
}

func newRegistry() *registry {
	return &registry{tools: make(map[string]toolEntry)}
}

// loadRegistry restores the tool declarations cached by the tools plugin. The
// cache is validated on load; a corrupt entry is skipped rather than poisoning
// the registry. The runtime availability gate still checks every source.
func loadRegistry(path string) (*registry, error) {
	reg := newRegistry()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return reg, err
	}
	var persisted persistedRegistry
	if err := json.Unmarshal(data, &persisted); err != nil {
		return reg, err
	}
	for _, item := range persisted.Tools {
		definition := item.Definition
		if definition.SourcePlugin == "" || item.Generation < 0 {
			continue
		}
		checked, err := validateDefinition(RegisterInput{
			Name: definition.Name, Description: definition.Description,
			InputSchema: definition.InputSchema, Risk: definition.Risk, Tags: definition.Tags,
			TargetCapability: definition.TargetCapability, TargetMethod: definition.TargetMethod,
		})
		if err != nil {
			continue
		}
		checked.SourcePlugin = definition.SourcePlugin
		checked.SourceGeneration = item.Generation
		reg.tools[checked.Name] = toolEntry{definition: checked, generation: item.Generation}
	}
	return reg, nil
}

func (r *registry) save(path string) error {
	r.mu.RLock()
	persisted := persistedRegistry{Version: 1, Tools: make([]persistedTool, 0, len(r.tools))}
	for _, entry := range r.tools {
		persisted.Tools = append(persisted.Tools, persistedTool{
			Definition: entry.definition,
			Generation: entry.generation,
		})
	}
	r.mu.RUnlock()
	sort.Slice(persisted.Tools, func(i, j int) bool {
		return persisted.Tools[i].Definition.Name < persisted.Tools[j].Definition.Name
	})
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".registry-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// replaceSource atomically replaces every Tool owned by source.
func (r *registry) replaceSource(source string, definitions []Definition, generation int) (int, int, int, error) {
	incoming := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if _, exists := incoming[definition.Name]; exists {
			return 0, 0, 0, pluginsdk.ErrCode("tool_definition_invalid", "duplicate tool name: "+definition.Name)
		}
		definition.SourcePlugin = source
		definition.SourceGeneration = generation
		incoming[definition.Name] = definition
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for name := range incoming {
		if existing, exists := r.tools[name]; exists && existing.definition.SourcePlugin != source {
			return 0, 0, 0, pluginsdk.ErrCode("tool_name_conflict",
				fmt.Sprintf("tool %s is owned by %s", name, existing.definition.SourcePlugin))
		}
	}

	added, updated, removed := 0, 0, 0
	for name, entry := range r.tools {
		if entry.definition.SourcePlugin != source {
			continue
		}
		if _, kept := incoming[name]; !kept {
			delete(r.tools, name)
			removed++
		}
	}
	for name, definition := range incoming {
		if existing, exists := r.tools[name]; exists {
			if !sameDefinition(existing.definition, definition) {
				updated++
			}
			r.tools[name] = toolEntry{definition: definition, generation: generation}
			continue
		}
		r.tools[name] = toolEntry{definition: definition, generation: generation}
		added++
	}
	return added, updated, removed, nil
}

func sameDefinition(left, right Definition) bool {
	return left.Name == right.Name &&
		left.Description == right.Description &&
		string(left.InputSchema) == string(right.InputSchema) &&
		left.Risk == right.Risk &&
		left.TargetCapability == right.TargetCapability &&
		left.TargetMethod == right.TargetMethod &&
		equalStrings(left.Tags, right.Tags)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (r *registry) put(definition Definition, generation int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, found := r.tools[definition.Name]
	if found && existing.definition.SourcePlugin != definition.SourcePlugin {
		return false, pluginsdk.ErrCode("tool_name_conflict",
			fmt.Sprintf("tool %s is owned by %s", definition.Name, existing.definition.SourcePlugin))
	}
	definition.SourceGeneration = generation
	r.tools[definition.Name] = toolEntry{definition: definition, generation: generation}
	return found, nil
}

func (r *registry) remove(name, source string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, found := r.tools[name]
	if !found {
		return false, nil
	}
	if entry.definition.SourcePlugin != source {
		return false, pluginsdk.ErrCode("tool_owner_mismatch", "tool is owned by another plugin")
	}
	delete(r.tools, name)
	return true, nil
}

func (r *registry) get(name string) (toolEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.tools[name]
	return entry, ok
}

func (r *registry) snapshot() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedDefinitions(r.tools)
}

func (r *registry) available(host hostSnapshot) []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.tools))
	for _, entry := range r.tools {
		definition := entry.definition
		if ok, _ := host.availability(
			definition.SourcePlugin, definition.TargetCapability, definition.TargetMethod, entry.generation,
		); !ok {
			continue
		}
		definition.SourceGeneration = entry.generation
		out = append(out, definition)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedDefinitions(entries map[string]toolEntry) []Definition {
	out := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.definition)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func validateDefinition(in RegisterInput) (Definition, error) {
	if !validToolName(in.Name) {
		return Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "name must match ^[a-z][a-z0-9_]{0,63}$")
	}
	descriptionBytes := len([]byte(strings.TrimSpace(in.Description)))
	if descriptionBytes == 0 || descriptionBytes > 8192 {
		return Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "description must be 1 to 8192 bytes")
	}
	if in.Risk < 0 || in.Risk > MaxRisk {
		return Definition{}, pluginsdk.ErrCode("tool_definition_invalid",
			fmt.Sprintf("risk must be between 0 and %d", MaxRisk))
	}
	if !capabilityPattern.MatchString(in.TargetCapability) {
		return Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "invalid target_capability")
	}
	if in.TargetCapability == "host" || in.TargetCapability == "tools" {
		return Definition{}, pluginsdk.ErrCode("tool_target_reserved", "target capability is reserved")
	}
	if !targetMethodPattern.MatchString(in.TargetMethod) {
		return Definition{}, pluginsdk.ErrCode("tool_definition_invalid", "invalid target_method")
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
		Tags:             tags,
		TargetCapability: in.TargetCapability,
		TargetMethod:     in.TargetMethod,
	}, nil
}

func validateTags(tags []string) ([]string, error) {
	if len(tags) > 16 {
		return nil, pluginsdk.ErrCode("tool_definition_invalid", "at most 16 tags are allowed")
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if !tagPattern.MatchString(tag) {
			return nil, pluginsdk.ErrCode("tool_definition_invalid", "invalid tag: "+tag)
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
