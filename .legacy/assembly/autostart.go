package assembly

import (
	"fmt"
	"os"
	"sort"

	"github.com/tomori/my-go-lite-agent/discovery"
)

// ResolveClosure is the single dependency-closure algorithm for plugin trees:
// it expands roots over their dependsOn closure and returns the mount set.
// Unknown names go to missing (named directly by a root); native Command
// conflicts are rejected. Order is pre-order (a plugin before its deps).
func ResolveClosure(res discovery.Result, roots []string) (mounted []discovery.Found, missing []string, rejected []Rejected) {
	byName := make(map[string]discovery.Found, len(res.Plugins))
	for _, p := range res.Plugins {
		byName[p.Manifest.Name] = p
	}

	seen := map[string]bool{}
	var add func(name, via string)
	add = func(name, via string) {
		if seen[name] {
			return
		}
		seen[name] = true
		p, ok := byName[name]
		if !ok {
			if via != "" {
				fmt.Fprintf(os.Stderr, "dependsOn: %s requires unknown plugin %q (skipping)\n", via, name)
			} else {
				missing = append(missing, name)
			}
			return
		}
		if p.Manifest.ConflictsWithNativeCommand() {
			rejected = append(rejected, Rejected{
				Name:   name,
				Reason: fmt.Sprintf("plugin name %q conflicts with native command /%s", name, name),
			})
			return
		}
		mounted = append(mounted, p)
		for _, dep := range p.Manifest.DependsOn {
			if dep == name {
				fmt.Fprintf(os.Stderr, "dependsOn: %s references itself (skipping)\n", name)
				continue
			}
			add(dep, name)
		}
	}

	for _, r := range roots {
		add(r, "")
	}
	sort.Strings(missing)
	return mounted, missing, rejected
}

// ResolveAutostart builds the mount plan from Manifest autostart roots and
// dependsOn closures (ADR-0021). Cycles, unknown names, and native Command
// conflicts are handled by ResolveClosure; mount order is arbitrary (star
// routing). UI-only Plugins (no process) always mount so their Panel
// Components load.
func ResolveAutostart(res discovery.Result) Plan {
	var roots []string
	for _, p := range res.Plugins {
		if p.Manifest.Autostart {
			roots = append(roots, p.Manifest.Name)
		}
		if p.Manifest.Entry == "" && p.Manifest.UI != nil {
			roots = append(roots, p.Manifest.Name)
		}
	}
	mounted, missing, rejected := ResolveClosure(res, roots)

	plan := Plan{Mounted: mounted, Missing: missing, Rejected: rejected}
	mountedSet := make(map[string]bool, len(mounted))
	for _, p := range mounted {
		mountedSet[p.Manifest.Name] = true
	}
	for _, p := range res.Plugins {
		if !mountedSet[p.Manifest.Name] {
			plan.Unmounted = append(plan.Unmounted, p)
		}
	}
	return plan
}