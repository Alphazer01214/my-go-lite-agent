// Package discovery finds Plugins on disk. It only observes; it never starts processes.
package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tomori/my-go-lite-agent/plugin"
)

// Found is one discovered Plugin directory plus its validated manifest.
type Found struct {
	Dir      string
	Manifest plugin.Manifest
}

// Result is the outcome of a scan: valid plugins and per-directory errors.
type Result struct {
	Plugins []Found
	Errors  []ScanError
}

// ScanError records a directory that failed Discovery.
type ScanError struct {
	Dir string
	Err error
}

func (e ScanError) Error() string {
	return fmt.Sprintf("%s: %v", e.Dir, e.Err)
}

// Scan looks at <root>/<child>/plugin.json one level deep. It never launches Plugins.
func Scan(root string) Result {
	var res Result

	entries, err := os.ReadDir(root)
	if err != nil {
		res.Errors = append(res.Errors, ScanError{Dir: root, Err: fmt.Errorf("read plugins root: %w", err)})
		return res
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		manifestPath := filepath.Join(dir, "plugin.json")
		if _, err := os.Stat(manifestPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			res.Errors = append(res.Errors, ScanError{Dir: dir, Err: err})
			continue
		}

		m, err := plugin.LoadManifest(manifestPath)
		if err != nil {
			res.Errors = append(res.Errors, ScanError{Dir: dir, Err: err})
			continue
		}
		// UI-only Plugins have no executable; the UI Entry check below still applies.
		if m.Entry != "" {
			if err := m.EntryExists(dir); err != nil {
				res.Errors = append(res.Errors, ScanError{Dir: dir, Err: err})
				continue
			}
		}
		if m.UI != nil {
			if err := m.UIEntryExists(dir); err != nil {
				res.Errors = append(res.Errors, ScanError{Dir: dir, Err: err})
				continue
			}
			if err := m.UIAssetsExist(dir); err != nil {
				res.Errors = append(res.Errors, ScanError{Dir: dir, Err: err})
				continue
			}
		}
		// Plugin Readme is a development convention (free-form): warn only, never reject.
		if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil && os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "discover warn: plugin %s has no README.md (recommended)\n", m.Name)
		}
		// protocol=1 is still mountable (ADR-0007); Host drops unknown render kinds.
		// res.Plugins = append(res.Plugins, Found{Dir: dir, Manifest: *m})
	}

	sort.Slice(res.Plugins, func(i, j int) bool {
		return res.Plugins[i].Manifest.Name < res.Plugins[j].Manifest.Name
	})
	return res
}
