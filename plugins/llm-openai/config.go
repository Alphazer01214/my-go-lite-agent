package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	configFile     = "config.json"
	defaultBaseURL = "https://api.deepseek.com/v1"
	defaultModelID = "deepseek-chat"
)

// plugin holds the current config. complete captures a snapshot at call start.
type plugin struct {
	mu     sync.Mutex
	cfg    config
	active map[string]context.CancelFunc
	// cancelled remembers cancels that arrived before their completion
	// registered. Host does not order req and cancel on the wire, so this is a
	// real case; cancelledOrder keeps the buffer bounded.
	cancelled      map[string]bool
	cancelledOrder []string
}

// cancelledWindow bounds the pre-registration cancel buffer.
const cancelledWindow = 256

func newPlugin() *plugin {
	p := &plugin{active: make(map[string]context.CancelFunc), cancelled: make(map[string]bool)}
	p.cfg = loadConfig()
	return p
}

func loadConfig() config {
	cfg := config{
		BaseURL:      defaultBaseURL,
		Model:        defaultModelID,
		DefaultModel: defaultModelID,
		Models:       []modelConfig{{ID: defaultModelID, Label: defaultModelID}},
	}
	if data, err := os.ReadFile(configFile); err == nil {
		var c config
		if json.Unmarshal(data, &c) == nil {
			if c.BaseURL != "" {
				cfg.BaseURL = c.BaseURL
			}
			if c.APIKey != "" {
				cfg.APIKey = c.APIKey
			}
			cfg.Models = c.Models
			cfg.DefaultModel = c.DefaultModel
			if c.Model != "" {
				cfg.Model = c.Model
			}
		}
	}
	// env wins at startup
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
		cfg.Model = v
		cfg.DefaultModel = v
		cfg.Models = ensureModel(cfg.Models, v)
	}
	return normalizeConfig(cfg)
}

// normalizeConfig folds the legacy single-model field into a catalog and
// repairs whatever the file left inconsistent. Loading stays lenient: a bad
// entry is dropped instead of preventing the plugin from starting.
func normalizeConfig(c config) config {
	c.BaseURL = normalizeBaseURL(c.BaseURL)
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	models := sanitizeModels(c.Models)
	legacy := strings.TrimSpace(c.Model)
	if len(models) == 0 {
		id := legacy
		if id == "" {
			id = defaultModelID
		}
		models = []modelConfig{{ID: id, Label: id}}
	}
	def := strings.TrimSpace(c.DefaultModel)
	if def == "" && hasModel(models, legacy) {
		def = legacy
	}
	if def == "" || !hasModel(models, def) {
		def = models[0].ID
	}
	c.Models = models
	c.DefaultModel = def
	c.Model = def
	return c
}

func sanitizeModels(in []modelConfig) []modelConfig {
	out := make([]modelConfig, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, model := range in {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" || seen[model.ID] {
			continue
		}
		model.Label = strings.TrimSpace(model.Label)
		if model.Label == "" {
			model.Label = model.ID
		}
		if model.ContextWindow < 0 {
			model.ContextWindow = 0
		}
		seen[model.ID] = true
		out = append(out, model)
	}
	return out
}

func validateModels(in []modelConfig) ([]modelConfig, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("models must not be empty")
	}
	out := make([]modelConfig, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, model := range in {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			return nil, fmt.Errorf("model id is required")
		}
		if seen[model.ID] {
			return nil, fmt.Errorf("duplicate model id %q", model.ID)
		}
		if model.ContextWindow < 0 {
			return nil, fmt.Errorf("model %q context_window must not be negative", model.ID)
		}
		model.Label = strings.TrimSpace(model.Label)
		if model.Label == "" {
			model.Label = model.ID
		}
		seen[model.ID] = true
		out = append(out, model)
	}
	return out, nil
}

func ensureModel(models []modelConfig, id string) []modelConfig {
	if hasModel(models, id) {
		return models
	}
	return append(models, modelConfig{ID: id, Label: id})
}

func hasModel(models []modelConfig, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

func contextWindowFor(models []modelConfig, id string) int {
	for _, model := range models {
		if model.ID == id {
			return model.ContextWindow
		}
	}
	return 0
}

// normalizeBaseURL is applied on every input path (file, env, config.set) so
// the upstream URL is built the same way no matter where it came from.
func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func (p *plugin) registerCall(callID string, cancel context.CancelFunc) {
	if callID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelled[callID] {
		cancel()
		return
	}
	p.active[callID] = cancel
}

// cancelCall cancels one in-flight completion. It reports whether this call
// reached a live completion. An unknown id is parked in the bounded cancel
// buffer instead of being reported as cancelled: the completion may not have
// registered yet, and claiming success for a call that never existed would be
// a lie.
func (p *plugin) cancelCall(callID string) bool {
	if callID == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel, ok := p.active[callID]
	if !ok {
		p.rememberCancelLocked(callID)
		return false
	}
	cancel()
	delete(p.active, callID)
	return true
}

// rememberCancelLocked buffers one early cancel, evicting the oldest entry so a
// stream of cancels for calls that never start cannot grow the map forever.
func (p *plugin) rememberCancelLocked(callID string) {
	if p.cancelled[callID] {
		return
	}
	p.cancelled[callID] = true
	p.cancelledOrder = append(p.cancelledOrder, callID)
	for len(p.cancelledOrder) > cancelledWindow {
		oldest := p.cancelledOrder[0]
		p.cancelledOrder = p.cancelledOrder[1:]
		delete(p.cancelled, oldest)
	}
}

func (p *plugin) finishCall(callID string) {
	if callID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, callID)
	delete(p.cancelled, callID)
}

func (p *plugin) snapshot() config {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *plugin) getConfig() configGetOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getConfigLocked()
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

// configSet applies immediately. Running calls keep their captured snapshot.
func (p *plugin) configSet(in configSetIn) (configGetOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.applyLocked(in); err != nil {
		return configGetOut{}, errHandler(err.Error())
	}
	return p.getConfigLocked(), nil
}

func (p *plugin) getConfigLocked() configGetOut {
	return configGetOut{
		BaseURL:      p.cfg.BaseURL,
		APIKey:       maskKey(p.cfg.APIKey),
		Model:        p.cfg.DefaultModel,
		Models:       append([]modelConfig(nil), p.cfg.Models...),
		DefaultModel: p.cfg.DefaultModel,
	}
}

func (p *plugin) modelsList() modelsListOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.modelsListLocked()
}

func (p *plugin) modelsListLocked() modelsListOut {
	return modelsListOut{
		DefaultModel: p.cfg.DefaultModel,
		Models:       append([]modelConfig(nil), p.cfg.Models...),
	}
}

// modelsSet replaces the whole catalog. Unlike config.set, a model that is not
// listed is an error here: the manager UI is authoritative about the list.
func (p *plugin) modelsSet(in modelsSetIn) (modelsListOut, error) {
	models, err := validateModels(in.Models)
	if err != nil {
		return modelsListOut{}, errBadArguments(err.Error())
	}
	def := strings.TrimSpace(in.DefaultModel)
	if def == "" {
		def = models[0].ID
	}
	if !hasModel(models, def) {
		return modelsListOut{}, errBadArguments("default_model is not in models")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// A no-op save must not rewrite config.json unnecessarily.
	if p.cfg.DefaultModel == def && sameModels(p.cfg.Models, models) {
		return p.modelsListLocked(), nil
	}
	p.cfg.Models = models
	p.cfg.DefaultModel = def
	p.cfg.Model = def
	if err := saveConfig(p.cfg); err != nil {
		return modelsListOut{}, errHandler(err.Error())
	}
	return p.modelsListLocked(), nil
}

func sameModels(a, b []modelConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (p *plugin) applyLocked(in configSetIn) error {
	if in.BaseURL != "" {
		p.cfg.BaseURL = normalizeBaseURL(in.BaseURL)
	}
	if in.APIKey != "" {
		p.cfg.APIKey = in.APIKey
	}
	// Copy before appending: a completion that captured the previous config
	// holds the old slice and must not see a shared backing array change.
	models := append([]modelConfig(nil), p.cfg.Models...)
	if in.Models != nil {
		normalized, err := validateModels(*in.Models)
		if err != nil {
			return err
		}
		models = normalized
	}
	candidate := strings.TrimSpace(in.DefaultModel)
	legacy := strings.TrimSpace(in.Model)
	if candidate == "" {
		candidate = legacy
	}
	if candidate != "" && !hasModel(models, candidate) {
		models = append(models, modelConfig{ID: candidate, Label: candidate})
	}
	if candidate == "" {
		candidate = p.cfg.DefaultModel
	}
	if !hasModel(models, candidate) {
		candidate = models[0].ID
	}
	p.cfg.Models = models
	p.cfg.DefaultModel = candidate
	p.cfg.Model = candidate
	return saveConfig(p.cfg)
}

func saveConfig(c config) error {
	persisted := c
	persisted.Models = sanitizeModels(persisted.Models)
	if len(persisted.Models) == 0 {
		id := strings.TrimSpace(persisted.DefaultModel)
		if id == "" {
			id = defaultModelID
		}
		persisted.Models = []modelConfig{{ID: id, Label: id}}
	}
	if persisted.DefaultModel == "" || !hasModel(persisted.Models, persisted.DefaultModel) {
		persisted.DefaultModel = persisted.Models[0].ID
	}
	persisted.Model = persisted.DefaultModel
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0o644)
}
