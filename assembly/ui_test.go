package assembly

import (
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
)

func mounted(name string, mounts ...plugin.UIMount) discovery.Found {
	m := plugin.Manifest{
		Name:     name,
		Version:  "0.0.1",
		Protocol: plugin.CurrentProtocol,
		Entry:    name + ".exe",
		UI:       &plugin.UISpec{Entry: "main.js", Mounts: mounts},
	}
	return discovery.Found{Dir: name, Manifest: m}
}

func TestResolveUIMountsDefaults(t *testing.T) {
	plan := Plan{Mounted: []discovery.Found{
		mounted("session",
			plugin.UIMount{Slot: "left", Component: "session-rail"},
			plugin.UIMount{Slot: "center", Component: "session-workspace"},
			plugin.UIMount{Slot: "bottom", Component: "session-status"},
		),
	}}
	got, err := ResolveUIMounts(Config{}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	seen := map[string]string{}
	for _, g := range got {
		seen[g.Component] = g.Slot
	}
	if seen["session-rail"] != "left" || seen["session-workspace"] != "center" || seen["session-status"] != "bottom" {
		t.Fatalf("got=%+v", got)
	}
}

func TestResolveUIMountsDisableAndOverride(t *testing.T) {
	plan := Plan{Mounted: []discovery.Found{
		mounted("uidemo", plugin.UIMount{Slot: "left", Component: "uidemo-mode-panel"}),
		mounted("other", plugin.UIMount{Slot: "left", Component: "other-panel"}),
	}}
	cfg := Config{UI: &UIConfig{
		Disable: []string{"uidemo/uidemo-mode-panel"},
		Overrides: []UIOverride{{
			Plugin: "other", Component: "other-panel", Slot: "right",
			Props: json.RawMessage(`{"a":1}`), Winner: true,
		}},
	}}
	got, err := ResolveUIMounts(cfg, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Slot != "right" || string(got[0].Props) != `{"a":1}` {
		t.Fatalf("got=%+v", got[0])
	}
}

func TestResolveUIMountsWinnerDropsOthers(t *testing.T) {
	plan := Plan{Mounted: []discovery.Found{
		mounted("session", plugin.UIMount{Slot: "center", Component: "session-workspace"}),
		mounted("alt", plugin.UIMount{Slot: "center", Component: "alt-workspace"}),
	}}
	ord := 0
	cfg := Config{UI: &UIConfig{Overrides: []UIOverride{{
		Plugin: "alt", Component: "alt-workspace", Winner: true, Order: &ord,
	}}}}
	got, err := ResolveUIMounts(cfg, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Component != "alt-workspace" {
		t.Fatalf("got=%+v", got)
	}
}
