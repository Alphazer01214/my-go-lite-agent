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
			plugin.UIMount{Slot: "chat", Component: "session-view"},
			plugin.UIMount{Page: "trace", Slot: "main", Component: "session-trace"},
		),
	}}
	got, err := ResolveUIMounts(Config{}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Page != "main" || got[0].Component != "session-view" {
		t.Fatalf("got[0]=%+v", got[0])
	}
}

func TestResolveUIMountsDisableAndOverride(t *testing.T) {
	plan := Plan{Mounted: []discovery.Found{
		mounted("uidemo", plugin.UIMount{Slot: "sidebar", Component: "uidemo-mode-panel"}),
		mounted("other", plugin.UIMount{Slot: "sidebar", Component: "other-panel"}),
	}}
	cfg := Config{UI: &UIConfig{
		Disable: []string{"uidemo/uidemo-mode-panel"},
		Overrides: []UIOverride{{
			Plugin: "other", Component: "other-panel", Slot: "toolbar-right",
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
	if got[0].Slot != "toolbar-right" || string(got[0].Props) != `{"a":1}` {
		t.Fatalf("got=%+v", got[0])
	}
}

func TestResolveUIMountsWinnerDropsOthers(t *testing.T) {
	plan := Plan{Mounted: []discovery.Found{
		mounted("session", plugin.UIMount{Slot: "chat", Component: "session-view"}),
		mounted("alt", plugin.UIMount{Slot: "chat", Component: "alt-view"}),
	}}
	ord := 0
	cfg := Config{UI: &UIConfig{Overrides: []UIOverride{{
		Plugin: "alt", Component: "alt-view", Winner: true, Order: &ord,
	}}}}
	got, err := ResolveUIMounts(cfg, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Component != "alt-view" {
		t.Fatalf("got=%+v", got)
	}
}
