package host

import (
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func TestPluginSnapshotReportsLifecycleStates(t *testing.T) {
	h := newTestHost()
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{
		{Manifest: plugin.Manifest{
			Name: "mounted", Version: "1.0.0", Entry: "mounted", Autostart: true,
			Provides: []string{"one", "tools"},
		}},
		{Manifest: plugin.Manifest{
			Name: "degraded", Version: "2.0.0", Entry: "degraded", Autostart: true,
		}},
		{Manifest: plugin.Manifest{
			Name: "available", Version: "3.0.0", Entry: "available",
		}},
		{Manifest: plugin.Manifest{
			Name: "disabled", Version: "4.0.0", Entry: "disabled", Autostart: true,
		}},
		{Manifest: plugin.Manifest{
			Name: "missing-user", Version: "5.0.0", Entry: "missing-user", Autostart: true,
			DependsOn: []string{"missing"},
		}},
	}}
	h.plugins["mounted"] = &proc{healthy: true}
	h.plugins["degraded"] = &proc{healthy: false}
	h.disabled["disabled"] = true

	got := h.PluginSnapshot()
	want := []PluginSnapshot{
		{Name: "available", Version: "3.0.0", State: PluginStateAvailable, Enabled: true},
		{Name: "degraded", Version: "2.0.0", State: PluginStateDegraded, Enabled: true},
		{Name: "disabled", Version: "4.0.0", State: PluginStateDisabled, Enabled: false},
		{Name: "missing", State: PluginStateMissing, Enabled: true},
		{Name: "missing-user", Version: "5.0.0", State: PluginStateDegraded, Enabled: true, DependsOn: []string{"missing"}},
		{Name: "mounted", Version: "1.0.0", State: PluginStateMounted, Enabled: true, Provides: []string{"one", "tools"}},
	}
	assertPluginSnapshot(t, got, want)
}

func TestPluginSnapshotTreatsFailedPlannedLaunchAsDegraded(t *testing.T) {
	h := newTestHost()
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{
		{Manifest: plugin.Manifest{Name: "failed", Version: "1.0.0", Entry: "failed", Autostart: true}},
	}}

	got := h.PluginSnapshot()
	if len(got) != 1 || got[0].State != PluginStateDegraded {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestHostPluginsReturnsSnapshot(t *testing.T) {
	h := newTestHost("caller")
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{
		{Manifest: plugin.Manifest{Name: "demo", Version: "1.2.3", Entry: "demo", Autostart: true, Provides: []string{"demo"}}},
	}}
	h.plugins["demo"] = &proc{healthy: true}

	if err := h.hostPlugins("caller", &Frame{ID: "caller-1", Type: FrameRequest, Capability: HostCapability, Method: HostMethodPlugins}); err != nil {
		t.Fatal(err)
	}

	frames := drainFrames(t, h, "caller")
	if len(frames) != 1 {
		t.Fatalf("frames = %#v", frames)
	}
	var payload struct {
		Plugins []PluginSnapshot `json:"plugins"`
	}
	if err := json.Unmarshal(frames[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	assertPluginSnapshot(t, payload.Plugins, []PluginSnapshot{
		{Name: "demo", Version: "1.2.3", State: PluginStateMounted, Enabled: true, Provides: []string{"demo"}},
	})
}
func assertPluginSnapshot(t *testing.T, got, want []PluginSnapshot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("snapshot length = %d, want %d\n got: %#v\nwant: %#v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Name != w.Name || g.Version != w.Version || g.State != w.State || g.Enabled != w.Enabled {
			t.Fatalf("snapshot[%d] = %#v, want %#v", i, g, w)
		}
		if !equalStrings(g.Provides, w.Provides) || !equalStrings(g.DependsOn, w.DependsOn) {
			t.Fatalf("snapshot[%d] = %#v, want %#v", i, g, w)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
