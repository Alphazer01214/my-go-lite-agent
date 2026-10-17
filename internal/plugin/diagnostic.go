package plugin

import "fmt"

// Diagnostic reports a non-fatal plugin discovery or manifest problem.
type Diagnostic struct {
	Plugin  string `json:"plugin"`
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("plugin=%s path=%s code=%s message=%s", d.Plugin, d.Path, d.Code, d.Message)
}

const (
	diagnosticRootReadFailed        = "plugin_root_read_failed"
	diagnosticManifestMissing       = "manifest_missing"
	diagnosticManifestRead          = "manifest_read_failed"
	diagnosticManifestJSON          = "manifest_invalid_json"
	diagnosticManifestUnknown       = "manifest_unknown_field"
	diagnosticManifestName          = "manifest_name_mismatch"
	diagnosticManifestUnusable      = "manifest_unusable"
	diagnosticPluginReadme          = "plugin_readme_missing"
	diagnosticUIUnknown             = "ui_unknown_field"
	diagnosticSettingsUnknown       = "settings_unknown_field"
	diagnosticSettingsMethodUnknown = "settings_method_unknown_field"
	diagnosticSettingsFieldUnknown  = "settings_field_unknown_field"
	diagnosticSettingsType          = "settings_unknown_type"
)
