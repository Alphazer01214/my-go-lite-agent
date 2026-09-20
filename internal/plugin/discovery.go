package plugin

import (
	"fmt"
	"os"
	"path/filepath"
)

// Discoveries represents the result of a plugin scan.
type Discoveries struct {
	Plugins []Discovery
	Errors  []ScannedError
	// Warns   []ScanWarn
}

// Discovery represents a plugin discovery.
type Discovery struct {
	Dir      string
	Manifest Manifest
}

type ScannedError struct {
	Dir string
	Err error
}

// type ScanWarn struct {
// 	Dir  string
// 	Warn string
// }

func (e ScannedError) String() string {
	return fmt.Sprintf("%s: %v\n", e.Dir, e.Err)
}

// ScanRoot root: plugins, entry: plugins/entry, manifest: plugins/entry/plugin.json
func ScanRoot(root string) Discoveries {
	var res Discoveries
	var plgs []Discovery
	var errs []ScannedError
	// var warns []ScanWarn
	entries, err := os.ReadDir(root)
	if err != nil {
		errs = append(errs, ScannedError{
			Dir: root,
			Err: fmt.Errorf("failed to read directory %s: %v\n", root, err),
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
			errs = append(errs, ScannedError{
				Dir: dir,
				Err: fmt.Errorf("no manifest file (plugin.json) in %s is found\n", pluginName),
			})
		}
		manifest, err := LoadManifest(maniPath)
		if err != nil {
			errs = append(errs, ScannedError{
				Dir: dir,
				Err: fmt.Errorf("failed to load manifest file (plugin.json) in %s: %v\n", pluginName, err),
			})
			continue
		}

		if pluginName != manifest.Name {
			errs = append(errs, ScannedError{
				Dir: dir,
				Err: fmt.Errorf("plugin name in manifest (%s) does not match directory name (%s)\n", manifest.Name, pluginName),
			})
			continue
		}

		if _, err := os.Stat(readmePath); err != nil {
			// warns = append(warns, ScanWarn{
			// 	Dir:  dir,
			// 	Warn: fmt.Sprintf("No readme file (README.md) in %s is found.", pluginName),
			// })
			_, _ = fmt.Fprintf(os.Stderr, "no README.md in plugin %v\n", pluginName)
		}

		plgs = append(plgs, Discovery{
			Dir:      dir,
			Manifest: manifest,
		})
	}

	res.Plugins = plgs
	res.Errors = errs
	// res.Warns = warns
	return res
}
