package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type Manifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Protocol    int    `json:"protocol"`
	Description string `json:"description,omitempty"`
	Autostart   bool   `json:"autostart,omitempty"`
	Entry       string `json:"entry"`

	Priority  int              `json:"priority,omitempty"`
	Provides  []string         `json:"provides,omitempty"`
	Requires  []string         `json:"requires,omitempty"`
	DependsOn []string         `json:"depends_on,omitempty"`
	Consumes  []ConsumesMethod `json:"consumes,omitempty"`

	Commands []Command         `json:"commands,omitempty"`
	UI       *UIManifest       `json:"ui,omitempty"`
	Settings *SettingsManifest `json:"settings,omitempty"`
}

type ConsumesMethod struct {
	Capability string `json:"capability"`
	Method     string `json:"method"`
}

type UIManifest struct {
	Entry      string        `json:"entry"`
	Components []UIComponent `json:"components,omitempty"`
}

type UIComponent struct {
	Tag     string `json:"tag"`
	MountID string `json:"mount_id"`
	Slot    string `json:"slot"`
}

type SettingsManifest struct {
	Get    SettingsMethod  `json:"get"`
	Save   SettingsMethod  `json:"save"`
	Schema []SettingsField `json:"schema,omitempty"`
}

type SettingsMethod struct {
	Capability string `json:"capability"`
	Method     string `json:"method"`
}

type SettingsField struct {
	Key      string            `json:"key"`
	Label    string            `json:"label,omitempty"`
	Type     SettingsFieldType `json:"type"`
	Required bool              `json:"required,omitempty"`
}

type SettingsFieldType string

const (
	SettingsFieldString  SettingsFieldType = "string"
	SettingsFieldSecret  SettingsFieldType = "secret"
	SettingsFieldNumber  SettingsFieldType = "number"
	SettingsFieldBoolean SettingsFieldType = "boolean"
)

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Usage       string `json:"usage,omitempty"`
}

var manifestFields = map[string]struct{}{
	"name": {}, "version": {}, "protocol": {}, "description": {},
	"autostart": {}, "entry": {}, "priority": {}, "provides": {}, "requires": {},
	"depends_on": {}, "consumes": {}, "commands": {}, "ui": {}, "settings": {},
}

var (
	uiFields = map[string]struct{}{
		"entry": {}, "components": {},
	}
	settingsFields = map[string]struct{}{
		"get": {}, "save": {}, "schema": {},
	}
	settingsMethodFields = map[string]struct{}{
		"capability": {}, "method": {},
	}
	settingsSchemaFields = map[string]struct{}{
		"key": {}, "label": {}, "type": {}, "required": {},
	}
)

func LoadManifest(path string) (Manifest, []Diagnostic) {
	pluginName := filepath.Base(filepath.Dir(filepath.Clean(path)))
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, []Diagnostic{{
			Plugin: pluginName, Path: path, Code: diagnosticManifestRead, Message: err.Error(),
		}}
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		message := "manifest must be a JSON object"
		if err != nil {
			message = err.Error()
		}
		return Manifest{}, []Diagnostic{{
			Plugin: pluginName, Path: path, Code: diagnosticManifestJSON, Message: message,
		}}
	}

	diagnostics := unknownFieldDiagnostics(pluginName, path, fields, manifestFields, diagnosticManifestUnknown)
	diagnostics = append(diagnostics, nestedManifestDiagnostics(pluginName, path, fields)...)

	var res Manifest
	if err := json.Unmarshal(data, &res); err != nil {
		return Manifest{}, append(diagnostics, Diagnostic{
			Plugin: pluginName, Path: path, Code: diagnosticManifestJSON, Message: err.Error(),
		})
	}
	return res, diagnostics
}

func nestedManifestDiagnostics(pluginName, path string, fields map[string]json.RawMessage) []Diagnostic {
	var diagnostics []Diagnostic

	if raw, ok := fields["ui"]; ok {
		uiPath := path + "#ui"
		if object, err := objectFields(raw); err == nil && object != nil {
			diagnostics = append(diagnostics, unknownFieldDiagnostics(pluginName, uiPath, object, uiFields, diagnosticUIUnknown)...)
		}
	}

	rawSettings, ok := fields["settings"]
	if !ok {
		return diagnostics
	}
	settingsPath := path + "#settings"
	settings, err := objectFields(rawSettings)
	if err != nil || settings == nil {
		return diagnostics
	}
	diagnostics = append(diagnostics, unknownFieldDiagnostics(pluginName, settingsPath, settings, settingsFields, diagnosticSettingsUnknown)...)

	for _, name := range []string{"get", "save"} {
		rawMethod, ok := settings[name]
		if !ok {
			continue
		}
		methodPath := settingsPath + "." + name
		if method, err := objectFields(rawMethod); err == nil && method != nil {
			diagnostics = append(diagnostics, unknownFieldDiagnostics(
				pluginName, methodPath, method, settingsMethodFields, diagnosticSettingsMethodUnknown,
			)...)
		}
	}

	rawSchema, ok := settings["schema"]
	if !ok {
		return diagnostics
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(rawSchema, &entries); err != nil {
		return diagnostics
	}
	for i, rawEntry := range entries {
		entryPath := settingsPath + ".schema[" + strconv.Itoa(i) + "]"
		entry, err := objectFields(rawEntry)
		if err != nil || entry == nil {
			continue
		}
		diagnostics = append(diagnostics, unknownFieldDiagnostics(
			pluginName, entryPath, entry, settingsSchemaFields, diagnosticSettingsFieldUnknown,
		)...)
		if rawType, ok := entry["type"]; ok {
			var fieldType SettingsFieldType
			if err := json.Unmarshal(rawType, &fieldType); err == nil && !validSettingsFieldType(fieldType) {
				diagnostics = append(diagnostics, Diagnostic{
					Plugin: pluginName, Path: entryPath, Code: diagnosticSettingsType,
					Message: "unknown settings type: " + string(fieldType),
				})
			}
		}
	}
	return diagnostics
}

func objectFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func unknownFieldDiagnostics(
	pluginName, path string,
	fields map[string]json.RawMessage,
	known map[string]struct{},
	code string,
) []Diagnostic {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var diagnostics []Diagnostic
	for _, key := range keys {
		if _, ok := known[key]; ok {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Plugin: pluginName, Path: path, Code: code,
			Message: "unknown field: " + key,
		})
	}
	return diagnostics
}

func validSettingsFieldType(fieldType SettingsFieldType) bool {
	switch fieldType {
	case SettingsFieldString, SettingsFieldSecret, SettingsFieldNumber, SettingsFieldBoolean:
		return true
	default:
		return false
	}
}
