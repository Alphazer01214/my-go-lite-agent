package assembly

import (
	"testing"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
)

func TestResolveAutostartClosure(t *testing.T) {
	res := discovery.Result{Plugins: []discovery.Found{
		{Dir: "/p/agent", Manifest: plugin.Manifest{Name: "agent", Autostart: true, DependsOn: []string{"session"}, Entry: "a"}},
		{Dir: "/p/session", Manifest: plugin.Manifest{Name: "session", Entry: "s"}},
		{Dir: "/p/filetools", Manifest: plugin.Manifest{Name: "filetools", Entry: "f"}},
	}}
	plan := ResolveAutostart(res)
	if len(plan.Mounted) != 2 {
		t.Fatalf("mounted: %+v", plan.Mounted)
	}
	names := map[string]bool{}
	for _, p := range plan.Mounted {
		names[p.Manifest.Name] = true
	}
	if !names["agent"] || !names["session"] {
		t.Fatalf("want agent+session: %v", names)
	}
	if len(plan.Unmounted) != 1 || plan.Unmounted[0].Manifest.Name != "filetools" {
		t.Fatalf("unmounted: %+v", plan.Unmounted)
	}
}

func TestResolveAutostartCycle(t *testing.T) {
	res := discovery.Result{Plugins: []discovery.Found{
		{Dir: "/p/a", Manifest: plugin.Manifest{Name: "alpha", Autostart: true, DependsOn: []string{"beta"}, Entry: "a"}},
		{Dir: "/p/b", Manifest: plugin.Manifest{Name: "beta", DependsOn: []string{"alpha"}, Entry: "b"}},
	}}
	plan := ResolveAutostart(res)
	if len(plan.Mounted) != 2 {
		t.Fatalf("cycle should still mount both: %+v", plan.Mounted)
	}
}

func TestResolveAutostartIncludesUIOnly(t *testing.T) {
	res := discovery.Result{Plugins: []discovery.Found{
		{Dir: "/p/agent", Manifest: plugin.Manifest{Name: "agent", Autostart: true, Entry: "a"}},
		{Dir: "/p/uiview", Manifest: plugin.Manifest{
			Name: "uiview",
			UI:   &plugin.UISpec{Entry: "main.js"},
		}},
	}}
	plan := ResolveAutostart(res)
	names := map[string]bool{}
	for _, p := range plan.Mounted {
		names[p.Manifest.Name] = true
	}
	if !names["agent"] || !names["uiview"] {
		t.Fatalf("want agent+uiview mounted: %v", names)
	}
}

func TestExpandDepends(t *testing.T) {
	res := discovery.Result{Plugins: []discovery.Found{
		{Dir: "/p/f", Manifest: plugin.Manifest{Name: "filetools", DependsOn: []string{"session"}, Entry: "f"}},
		{Dir: "/p/s", Manifest: plugin.Manifest{Name: "session", Entry: "s"}},
	}}
	names, missing := ExpandDepends(res, []string{"filetools", "ghost"})
	if len(names) != 2 || len(missing) != 1 || missing[0] != "ghost" {
		t.Fatalf("names=%v missing=%v", names, missing)
	}
}
