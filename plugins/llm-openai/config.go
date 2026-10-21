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

// loadConfig builds the runtime catalog from config.json and the environment.
// Every model carries its own endpoint, so one catalog can hold models from
// several providers. Legacy single-endpoint files migrate on the way in: the
// top-level base_url and api_key fill entries that carry none of their own.
func loadConfig() config {
	file := config{
		Model:        defaultModelID,
		DefaultModel: defaultModelID,
		Models:       []modelConfig{{ID: defaultModelID, Label: defaultModelID}},
	}
	if data, err := os.ReadFile(configFile); err == nil {
		var c config
		if json.Unmarshal(data, &c) == nil {
			// The file wins over the built-in defaults wholesale; normalizeConfig
			// fills whatever it left empty.
			file = c
			if file.Model == "" {
				file.Model = defaultModelID
			}
		}
	}
	cfg := normalizeConfig(file)

	// env wins at startup. One base URL and one key cannot describe several
	// providers, so they address the default model's entry only.
	models := append([]modelConfig(nil), cfg.Models...)
	def := cfg.DefaultModel
	if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
		def = v
		if !hasModel(models, v) {
			models = append(models, modelConfig{ID: v, Label: v})
		}
	}
	if entry := findModel(models, def); entry != nil {
		if v := normalizeBaseURL(os.Getenv("OPENAI_BASE_URL")); v != "" {
			entry.BaseURL = v
		}
		if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
			entry.APIKey = v
		}
	}
	return normalizeConfig(config{Models: models, DefaultModel: def})
}

// normalizeConfig folds the legacy single-endpoint fields into per-model
// endpoints and repairs whatever the file left inconsistent. Loading stays
// lenient: a bad entry is dropped instead of preventing the plugin from
// starting. The legacy top-level fields end up mirroring the default entry so
// llm.config.get keeps its old shape.
func normalizeConfig(c config) config {
	legacyBase := normalizeBaseURL(c.BaseURL)
	if legacyBase == "" {
		legacyBase = defaultBaseURL
	}
	models := sanitizeModels(c.Models, legacyBase, strings.TrimSpace(c.APIKey))
	legacy := strings.TrimSpace(c.Model)
	if len(models) == 0 {
		id := legacy
		if id == "" {
			id = defaultModelID
		}
		models = []modelConfig{{ID: id, Label: id, BaseURL: legacyBase, APIKey: strings.TrimSpace(c.APIKey)}}
	}
	def := strings.TrimSpace(c.DefaultModel)
	if def == "" && hasModel(models, legacy) {
		def = legacy
	}
	if def == "" || !hasModel(models, def) {
		def = models[0].ID
	}
	entry := findModel(models, def)
	c.Models = models
	c.DefaultModel = def
	c.Model = def
	c.BaseURL = entry.BaseURL
	c.APIKey = entry.APIKey
	return c
}

// sanitizeModels trims and dedupes entries, filling an entry's missing
// endpoint from the legacy top-level values (or the built-in default).
func sanitizeModels(in []modelConfig, legacyBase, legacyKey string) []modelConfig {
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
		model.BaseURL = normalizeBaseURL(model.BaseURL)
		if model.BaseURL == "" {
			model.BaseURL = legacyBase
		}
		if model.BaseURL == "" {
			model.BaseURL = defaultBaseURL
		}
		model.APIKey = strings.TrimSpace(model.APIKey)
		if model.APIKey == "" {
			model.APIKey = strings.TrimSpace(legacyKey)
		}
		seen[model.ID] = true
		out = append(out, model)
	}
	return out
}

// validateModels normalizes input entries against the stored catalog. Unlike
// sanitizeModels it refuses to guess: a new model id must carry its own
// endpoint. An entry that leaves base_url empty keeps the endpoint stored
// under the same id, so a metadata-only update does not have to resend it.
func validateModels(in []modelEntryIn, old []modelConfig) ([]modelConfig, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("models must not be empty")
	}
	out := make([]modelConfig, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, entry := range in {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			return nil, fmt.Errorf("model id is required")
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate model id %q", id)
		}
		if entry.ContextWindow < 0 {
			return nil, fmt.Errorf("model %q context_window must not be negative", id)
		}
		label := strings.TrimSpace(entry.Label)
		if label == "" {
			label = id
		}
		base := normalizeBaseURL(entry.BaseURL)
		if base == "" {
			if prev := findModel(old, id); prev != nil {
				base = prev.BaseURL
			}
		}
		if base == "" {
			return nil, fmt.Errorf("model %q base_url is required", id)
		}
		seen[id] = true
		out = append(out, modelConfig{
			ID:            id,
			Label:         label,
			ContextWindow: entry.ContextWindow,
			BaseURL:       base,
			APIKey:        resolveAPIKey(entry.APIKey, id, old),
		})
	}
	return out, nil
}

// resolveAPIKey turns one input api_key into the stored key. Absent keeps the
// key stored under the same id, "" clears it, and any other value is the new
// key. A masked echo — anything containing the mask marker — is never stored:
// it keeps the key it was made from, matched against the whole previous
// catalog so a renamed row does not lose its key. A masked value that matches
// nothing clears the key rather than persisting a placeholder.
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

func findModel(models []modelConfig, id string) *modelConfig {
	for i := range models {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

func hasModel(models []modelConfig, id string) bool {
	return findModel(models, id) != nil
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

// maskedModels copies the catalog with every key masked. The masked value is
// what llm.config.get and llm.settings.get hand out; a caller that sends it
// back unchanged keeps the stored key.
func maskedModels(models []modelConfig) []modelConfig {
	out := make([]modelConfig, 0, len(models))
	for _, model := range models {
		model.APIKey = maskKey(model.APIKey)
		out = append(out, model)
	}
	return out
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
		Models:       maskedModels(p.cfg.Models),
		DefaultModel: p.cfg.DefaultModel,
	}
}

func (p *plugin) modelsList() modelsListOut {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.modelsListLocked()
}

func (p *plugin) modelsListLocked() modelsListOut {
	models := make([]modelSummary, 0, len(p.cfg.Models))
	for _, model := range p.cfg.Models {
		models = append(models, modelSummary{
			ID:            model.ID,
			Label:         model.Label,
			ContextWindow: model.ContextWindow,
		})
	}
	return modelsListOut{DefaultModel: p.cfg.DefaultModel, Models: models}
}

// modelsSet replaces the whole catalog. Unlike config.set, a model that is not
// listed is an error here: the manager UI is authoritative about the list.
func (p *plugin) modelsSet(in modelsSetIn) (modelsListOut, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	models, err := validateModels(in.Models, p.cfg.Models)
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
	// A no-op save must not rewrite config.json unnecessarily.
	if p.cfg.DefaultModel == def && sameModels(p.cfg.Models, models) {
		return p.modelsListLocked(), nil
	}
	p.cfg = normalizeConfig(config{Models: models, DefaultModel: def})
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

// applyLocked merges one config.set payload. The payload stays lenient where
// the manager methods are strict: a default outside the catalog becomes a new
// entry that inherits the previous default's endpoint, and the legacy top-level
// base_url/api_key fill entries that carry none of their own. Nothing is
// applied when validation fails.
func (p *plugin) applyLocked(in configSetIn) error {
	old := p.cfg.Models
	models := append([]modelConfig(nil), old...)
	if in.Models != nil {
		normalized, err := validateModels(inheritEndpoint(*in.Models, in.BaseURL, in.APIKey), old)
		if err != nil {
			return err
		}
		models = normalized
	}
	candidate := strings.TrimSpace(in.DefaultModel)
	if candidate == "" {
		candidate = strings.TrimSpace(in.Model)
	}
	if candidate == "" {
		candidate = p.cfg.DefaultModel
	}
	if !hasModel(models, candidate) {
		// A default outside the catalog becomes a new entry that inherits the
		// previous default's endpoint, so `config.set model=x` keeps working.
		entry := modelConfig{
			ID:      candidate,
			Label:   candidate,
			BaseURL: normalizeBaseURL(in.BaseURL),
			APIKey:  strings.TrimSpace(in.APIKey),
		}
		if entry.BaseURL == "" || entry.APIKey == "" {
			if prev := findModel(old, p.cfg.DefaultModel); prev != nil {
				if entry.BaseURL == "" {
					entry.BaseURL = prev.BaseURL
				}
				if entry.APIKey == "" {
					entry.APIKey = prev.APIKey
				}
			}
		}
		models = append(models, entry)
	} else if in.Models == nil && (in.BaseURL != "" || strings.TrimSpace(in.APIKey) != "") {
		// Legacy shape: one endpoint for the catalog is expressed as an update
		// to the default model's entry.
		entry := findModel(models, candidate)
		if in.BaseURL != "" {
			entry.BaseURL = normalizeBaseURL(in.BaseURL)
		}
		if key := strings.TrimSpace(in.APIKey); key != "" {
			entry.APIKey = key
		}
	}
	p.cfg = normalizeConfig(config{Models: models, DefaultModel: candidate})
	return saveConfig(p.cfg)
}

// inheritEndpoint fills input entries that carry no endpoint of their own with
// the payload's legacy top-level base_url/api_key. An entry that does state its
// endpoint always wins, so one payload can mix several providers.
func inheritEndpoint(in []modelEntryIn, baseURL, apiKey string) []modelEntryIn {
	baseURL = normalizeBaseURL(baseURL)
	key := strings.TrimSpace(apiKey)
	if baseURL == "" && key == "" {
		return in
	}
	out := append([]modelEntryIn(nil), in...)
	for i := range out {
		if out[i].BaseURL == "" {
			out[i].BaseURL = baseURL
		}
		if out[i].APIKey == nil && key != "" {
			out[i].APIKey = &key
		}
	}
	return out
}

// saveConfig persists the multi-provider shape: one entry per model with its
// own endpoint and key. The legacy top-level fields are not written; they only
// exist as migration input and as llm.config.get mirrors.
func saveConfig(c config) error {
	persisted := config{
		Models:       sanitizeModels(c.Models, "", ""),
		DefaultModel: c.DefaultModel,
	}
	if len(persisted.Models) == 0 {
		id := strings.TrimSpace(persisted.DefaultModel)
		if id == "" {
			id = defaultModelID
		}
		persisted.Models = []modelConfig{{ID: id, Label: id, BaseURL: defaultBaseURL}}
	}
	if persisted.DefaultModel == "" || !hasModel(persisted.Models, persisted.DefaultModel) {
		persisted.DefaultModel = persisted.Models[0].ID
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile, data, 0o644)
}
