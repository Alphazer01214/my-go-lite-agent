package host

import (
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func TestPluginSnapshotReportsProviderLifecycleStates(t *testing.T) {
	h := newTestHost()
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{
		{Manifest: plugin.Manifest{Name: "inactive", Version: "3.0.0", Entry: "inactive", Autostart: true}},
		{Manifest: plugin.Manifest{Name: "failed", Version: "2.0.0", Entry: "failed", Autostart: true}},
		{Manifest: plugin.Manifest{Name: "disabled", Version: "4.0.0", Entry: "disabled", Autostart: true}},
		{Manifest: plugin.Manifest{Name: "missing-user", Version: "5.0.0", Entry: "missing-user", DependsOn: []string{"missing"}}},
		{Manifest: plugin.Manifest{Name: "active", Version: "1.0.0", Description: "Active plugin", Entry: "active", Autostart: true, Provides: []string{"one"}}},
	}}
	h.plugins["active"] = &proc{instanceID: "active", pluginName: "active", state: ProviderStateActive, gen: 1}
	h.plugins["failed"] = &proc{instanceID: "failed", pluginName: "failed", state: ProviderStateFailed, gen: 1, failureCode: CodePluginDown}
	h.disabled["disabled"] = true

	got := h.PluginSnapshot()
	want := []PluginSnapshot{
		{Name: "active", Version: "1.0.0", Description: "Active plugin", State: ProviderStateActive, SelectedInstanceID: "active", Enabled: true, Provides: []string{"one"}},
		{Name: "disabled", Version: "4.0.0", State: ProviderStateInactive, Enabled: false},
		{Name: "failed", Version: "2.0.0", State: ProviderStateFailed, SelectedInstanceID: "failed", Enabled: true},
		{Name: "inactive", Version: "3.0.0", State: ProviderStateInactive, Enabled: true},
		{Name: "missing", State: ProviderStateInactive, Enabled: true, Missing: true},
		{Name: "missing-user", Version: "5.0.0", State: ProviderStateInactive, Enabled: true, DependsOn: []string{"missing"}},
	}
	assertPluginSnapshot(t, got, want)
}

func TestHostPluginsReturnsGroupedProviderSnapshot(t *testing.T) {
	h := newTestHost("caller")
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{
		{Manifest: plugin.Manifest{Name: "demo", Version: "1.2.3", Entry: "demo", Autostart: true, Provides: []string{"demo"}}},
	}}
	h.plugins["demo-a"] = &proc{instanceID: "demo-a", pluginName: "demo", state: ProviderStateActive, gen: 1}
	h.plugins["demo-b"] = &proc{instanceID: "demo-b", pluginName: "demo", state: ProviderStateLoading, gen: 2}

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
	if len(payload.Plugins) != 1 || len(payload.Plugins[0].Instances) != 2 {
		t.Fatalf("snapshot = %#v", payload.Plugins)
	}
	if payload.Plugins[0].SelectedInstanceID != "demo-a" {
		t.Fatalf("snapshot = %#v", payload.Plugins[0])
	}
}

func assertPluginSnapshot(t *testing.T, got, want []PluginSnapshot) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("snapshot length = %d, want %d\n got: %#v\nwant: %#v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Name != w.Name || g.Version != w.Version || g.Description != w.Description ||
			g.State != w.State || g.Enabled != w.Enabled ||
			g.Missing != w.Missing || g.SelectedInstanceID != w.SelectedInstanceID {
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
