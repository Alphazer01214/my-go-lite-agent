package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

type fakePluginStateProvider struct {
	states []host.PluginSnapshot
}

func (f fakePluginStateProvider) PluginSnapshot() []host.PluginSnapshot {
	return append([]host.PluginSnapshot(nil), f.states...)
}

func TestBootstrapIncludesPluginStatusSnapshot(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	server := NewServer(registry, nil)
	server.SetPluginStateProvider(fakePluginStateProvider{states: []host.PluginSnapshot{
		{Name: "demo", Version: "1.0.0", State: host.PluginStateMounted, Enabled: true, Provides: []string{"demo"}},
		{Name: "missing", State: host.PluginStateMissing, Enabled: true},
	}})

	req := httptest.NewRequest(http.MethodGet, "/bridge/bootstrap", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var bootstrap Bootstrap
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Plugins) != 2 {
		t.Fatalf("plugins = %#v", bootstrap.Plugins)
	}
	demo := bootstrap.Plugins[0]
	if demo.Name != "demo" || demo.Version != "1.0.0" || demo.State != host.PluginStateMounted || !demo.Enabled {
		t.Fatalf("demo = %#v", demo)
	}
	if len(demo.Provides) != 1 || demo.Provides[0] != "demo" {
		t.Fatalf("demo provides = %#v", demo.Provides)
	}
	if len(demo.Mounts) != 1 || demo.Mounts[0] != (MountRef{Plugin: "demo", ID: "chat"}) {
		t.Fatalf("demo mounts = %#v", demo.Mounts)
	}
	if bootstrap.Plugins[1].Name != "missing" || bootstrap.Plugins[1].State != host.PluginStateMissing {
		t.Fatalf("missing = %#v", bootstrap.Plugins[1])
	}
}

func TestBootstrapPluginSnapshotFallsBackToDiscoveredState(t *testing.T) {
	discovery := testDiscovery(t)
	discovery.Manifest.Version = "2.3.4"
	discovery.Manifest.Provides = []string{"demo"}
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	server := NewServer(registry, nil)

	bootstrap := server.Bootstrap()
	if len(bootstrap.Plugins) != 1 {
		t.Fatalf("plugins = %#v", bootstrap.Plugins)
	}
	got := bootstrap.Plugins[0]
	if got.Name != "demo" || got.Version != "2.3.4" || got.State != host.PluginStateAvailable || !got.Enabled {
		t.Fatalf("plugin = %#v", got)
	}
}

func TestBootstrapIncludesSettingsManifest(t *testing.T) {
	discovery := testDiscovery(t)
	discovery.Manifest.Settings = &plugin.SettingsManifest{
		Get:  plugin.SettingsMethod{Capability: "demo", Method: "settings.get"},
		Save: plugin.SettingsMethod{Capability: "demo", Method: "settings.save"},
		Schema: []plugin.SettingsField{
			{Key: "endpoint", Label: "Endpoint", Type: plugin.SettingsFieldString, Required: true},
		},
	}
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	got := NewServer(registry, nil).Bootstrap()
	if len(got.Plugins) != 1 || got.Plugins[0].Settings == nil {
		t.Fatalf("bootstrap = %#v", got.Plugins)
	}
	settings := got.Plugins[0].Settings
	if settings.Get.Method != "settings.get" || len(settings.Schema) != 1 || settings.Schema[0].Key != "endpoint" {
		t.Fatalf("settings = %#v", settings)
	}
}
