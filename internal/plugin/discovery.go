package plugin

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScanResult represents the result of a plugin scan.
type ScanResult struct {
	Plugins []ScanPlugin
	Errors  []ScanError
	// Warns   []ScanWarn
}

type ScanPlugin struct {
	Dir      string
	Manifest Manifest
}

type ScanError struct {
	Dir string
	Err error
}

// type ScanWarn struct {
// 	Dir  string
// 	Warn string
// }

func (e ScanError) String() string {
	return fmt.Sprintf("%s: %v", e.Dir, e.Err)
}

// ScanRoot root: plugins, entry: plugins/entry, manifest: plugins/entry/plugin.json
func ScanRoot(root string) ScanResult {
	var res ScanResult
	var plgs []ScanPlugin
	var errs []ScanError
	// var warns []ScanWarn
	entries, err := os.ReadDir(root)
	if err != nil {
		errs = append(errs, ScanError{
			Dir: root,
			Err: fmt.Errorf("Failed to read directory %s: %v", root, err),
		})
		return res
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		dir := filepath.Join(root, pluginName)

		// load manifest
		maniPath := filepath.Join(dir, "plugin.json")
		readmePath := filepath.Join(dir, "README.md")
		if _, err := os.Stat(maniPath); err != nil {
			errs = append(errs, ScanError{
				Dir: dir,
				Err: fmt.Errorf("No manifest file (plugin.json) in %s is found.", pluginName),
			})
		}
		manifest, err := LoadManifest(maniPath)
		if err != nil {
			errs = append(errs, ScanError{
				Dir: dir,
				Err: fmt.Errorf("Failed to load manifest file (plugin.json) in %s: %v", pluginName, err),
			})
			continue
		}

		if pluginName != manifest.Name {
			errs = append(errs, ScanError{
				Dir: dir,
				Err: fmt.Errorf("Plugin name in manifest (%s) does not match directory name (%s)", manifest.Name, pluginName),
			})
			continue
		}

		if _, err := os.Stat(readmePath); err != nil {
			// warns = append(warns, ScanWarn{
			// 	Dir:  dir,
			// 	Warn: fmt.Sprintf("No readme file (README.md) in %s is found.", pluginName),
			// })
			fmt.Fprintf(os.Stderr, "No README.md in plugin %v/", pluginName)
		}

		plgs = append(plgs, ScanPlugin{
			Dir:      dir,
			Manifest: manifest,
		})
	}

	res.Plugins = plgs
	res.Errors = errs
	// res.Warns = warns
	return res
}
