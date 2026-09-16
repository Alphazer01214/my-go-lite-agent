package assembly

import (
	"fmt"
	"os"
	"sort"

	"github.com/tomori/my-go-lite-agent/discovery"
)

// ResolveAutostart builds the mount plan from Manifest autostart roots and
// dependsOn closures (ADR-0021). Cycles and unknown names warn and skip;
// mount order is arbitrary (star routing). Native Command conflicts reject.
func ResolveAutostart(res discovery.Result) Plan {
	byName := make(map[string]discovery.Found, len(res.Plugins))
	for _, p := range res.Plugins {
		byName[p.Manifest.Name] = p
	}

	var plan Plan
	mounted := map[string]bool{}
	var rejected []Rejected

	var add func(name string, via string)
	add = func(name string, via string) {
		if mounted[name] {
			return
		}
		p, ok := byName[name]
		if !ok {
			if via != "" {
				fmt.Fprintf(os.Stderr, "dependsOn: %s requires unknown plugin %q (skipping)\n", via, name)
			} else {
				plan.Missing = append(plan.Missing, name)
			}
			return
		}
		if p.Manifest.ConflictsWithNativeCommand() {
			reason := fmt.Sprintf("plugin name %q conflicts with native command /%s", name, name)
			for _, r := range rejected {
				if r.Name == name {
					return
				}
			}
			rejected = append(rejected, Rejected{Name: name, Reason: reason})
			return
		}
		mounted[name] = true
		plan.Mounted = append(plan.Mounted, p)
		for _, dep := range p.Manifest.DependsOn {
			if dep == name {
				fmt.Fprintf(os.Stderr, "dependsOn: %s references itself (skipping)\n", name)
				continue
			}
			if mounted[dep] {
				continue
			}
			add(dep, name)
		}
	}

	for _, p := range res.Plugins {
		if p.Manifest.Autostart {
			add(p.Manifest.Name, "")
		}
		// UI-only Plugins (no process) always mount so their Panel Components load.
		if p.Manifest.Entry == "" && p.Manifest.UI != nil {
			add(p.Manifest.Name, "")
		}
	}
	for _, p := range res.Plugins {
		if !mounted[p.Manifest.Name] {
			plan.Unmounted = append(plan.Unmounted, p)
		}
	}
	plan.Rejected = rejected
	sort.Slice(plan.Missing, func(i, j int) bool { return plan.Missing[i] < plan.Missing[j] })
	return plan
}

// ExpandDepends returns the set of plugin names to ensure for roots, including
// their dependsOn closure against the catalog. Unknown names go to missing.
func ExpandDepends(res discovery.Result, roots []string) (names []string, missing []string) {
	byName := make(map[string]discovery.Found, len(res.Plugins))
	for _, p := range res.Plugins {
		byName[p.Manifest.Name] = p
	}
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		p, ok := byName[name]
		if !ok {
			missing = append(missing, name)
			seen[name] = true
			return
		}
		seen[name] = true
		names = append(names, name)
		for _, dep := range p.Manifest.DependsOn {
			walk(dep)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	sort.Strings(missing)
	return names, missing
}
