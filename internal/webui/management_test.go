package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

type fakePluginLifecycle struct {
	discoveries plugin.Discoveries
	states      []host.PluginSnapshot
	rescans     int
	started     []string
	stopped     []string
	startErr    error
	stopErr     error
}

func (f *fakePluginLifecycle) PluginSnapshot() []host.PluginSnapshot {
	return append([]host.PluginSnapshot(nil), f.states...)
}

func (f *fakePluginLifecycle) RescanPlugins() (plugin.Discoveries, error) {
	f.rescans++
	return f.discoveries, nil
}

func (f *fakePluginLifecycle) StartPlugin(name string) error {
	f.started = append(f.started, name)
	return f.startErr
}

func (f *fakePluginLifecycle) StopPlugin(name string) error {
	f.stopped = append(f.stopped, name)
	return f.stopErr
}

func managementRequest(path string) *http.Request {
	return httptest.NewRequest(http.MethodPost, path, strings.NewReader(""))
}

func TestPluginRescanRefreshesRegistry(t *testing.T) {
	old := testDiscovery(t, plugin.UIComponent{Tag: "old-chat", MountID: "chat", Slot: "center"})
	old.Manifest.Name = "old"
	next := testDiscovery(t, plugin.UIComponent{Tag: "new-chat", MountID: "chat", Slot: "center"})
	next.Manifest.Name = "new"
	manager := &fakePluginLifecycle{
		discoveries: plugin.Discoveries{Plugins: []plugin.Discovery{next}},
		states: []host.PluginSnapshot{
			{Name: "new", State: host.PluginStateAvailable, Enabled: true},
		},
	}
	server := NewServer(mustRegistry(t, plugin.Discoveries{Plugins: []plugin.Discovery{old}}), nil)
	server.SetPluginManager(manager)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, managementRequest("/bridge/plugins/rescan"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var bootstrap Bootstrap
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	if manager.rescans != 1 || len(bootstrap.Plugins) != 1 || bootstrap.Plugins[0].Name != "new" {
		t.Fatalf("manager = %#v bootstrap = %#v", manager, bootstrap)
	}
	if len(bootstrap.Mounts) != 1 || bootstrap.Mounts[0].Tag != "new-chat" {
		t.Fatalf("mounts = %#v", bootstrap.Mounts)
	}
}

func TestPluginStartStopReturnsBootstrap(t *testing.T) {
	manager := &fakePluginLifecycle{states: []host.PluginSnapshot{{Name: "demo", State: host.PluginStateMounted, Enabled: true}}}
	server := NewServer(nil, nil)
	server.SetPluginManager(manager)

	for _, test := range []struct {
		path string
		want *[]string
	}{
		{"/bridge/plugins/demo/start", &manager.started},
		{"/bridge/plugins/demo/stop", &manager.stopped},
	} {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, managementRequest(test.path))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", test.path, rec.Code, rec.Body.String())
		}
		if len(*test.want) != 1 || (*test.want)[0] != "demo" {
			t.Fatalf("%s actions = %#v", test.path, *test.want)
		}
	}
}

func TestPluginActionErrorsAreStructured(t *testing.T) {
	manager := &fakePluginLifecycle{startErr: errors.New("dependency unavailable")}
	server := NewServer(nil, nil)
	server.SetPluginManager(manager)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, managementRequest("/bridge/plugins/demo/start"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != "plugin_action_failed" || !strings.Contains(payload.Error.Message, "dependency") {
		t.Fatalf("error = %#v", payload)
	}
}

func mustRegistry(t *testing.T, discoveries plugin.Discoveries) *Registry {
	t.Helper()
	registry, _ := NewRegistry(discoveries)
	return registry
}
