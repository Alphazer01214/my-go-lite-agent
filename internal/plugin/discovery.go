package plugin

import (
	"os"
	"path/filepath"
)

// Discoveries represents the non-fatal result of a plugin scan.
type Discoveries struct {
	Plugins     []Discovery
	Diagnostics []Diagnostic
}

// Discovery represents one usable plugin manifest and its package directory.
type Discovery struct {
	Dir      string
	Manifest Manifest
}

// ScanRoot scans root/plugin directories and reports all problems as diagnostics.
func ScanRoot(root string) Discoveries {
	var res Discoveries
	entries, err := os.ReadDir(root)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, Diagnostic{
			Path: root, Code: diagnosticRootReadFailed, Message: err.Error(),
		})
		return res
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		dir := filepath.Join(root, pluginName)
		manifestPath := filepath.Join(dir, "plugin.json")
		readmePath := filepath.Join(dir, "README.md")

		if _, err := os.Stat(manifestPath); err != nil {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Plugin: pluginName, Path: manifestPath,
				Code: diagnosticManifestMissing, Message: err.Error(),
			})
			continue
		}

		manifest, diagnostics := LoadManifest(manifestPath)
		res.Diagnostics = append(res.Diagnostics, diagnostics...)
		if manifest.Name == "" || manifest.Entry == "" {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Plugin: pluginName, Path: manifestPath, Code: diagnosticManifestUnusable,
				Message: "name and entry are required",
			})
			continue
		}

		if pluginName != manifest.Name {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Plugin: pluginName, Path: manifestPath, Code: diagnosticManifestName,
				Message: "manifest name does not match directory name",
			})
		}

		if _, err := os.Stat(readmePath); err != nil {
			res.Diagnostics = append(res.Diagnostics, Diagnostic{
				Plugin: pluginName, Path: readmePath, Code: diagnosticPluginReadme, Message: err.Error(),
			})
		}

		res.Plugins = append(res.Plugins, Discovery{Dir: dir, Manifest: manifest})
	}
	return res
}
