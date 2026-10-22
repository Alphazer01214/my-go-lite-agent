package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
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

func (r *registry) replaceSource(source string, definitions []Definition, generation int) (int, int, int, error) {
	incoming := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if _, exists := incoming[definition.Name]; exists {
			return 0, 0, 0, errToolDefinitionInvalid("duplicate tool name: " + definition.Name)
		}
		definition.SourcePlugin = source
		definition.SourceGeneration = generation
		incoming[definition.Name] = definition
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for name := range incoming {
		if existing, exists := r.tools[name]; exists && existing.definition.SourcePlugin != source {
			return 0, 0, 0, errToolNameConflict(fmt.Sprintf("tool %s is owned by %s", name, existing.definition.SourcePlugin))
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
		left.ParallelSafe == right.ParallelSafe &&
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
		return false, errToolNameConflict(fmt.Sprintf("tool %s is owned by %s", definition.Name, existing.definition.SourcePlugin))
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
		return false, errToolOwnerMismatch("tool is owned by another plugin")
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
