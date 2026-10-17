package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func TestHandlerServesShellBootstrapAndPlatformAssets(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	handler := NewServer(registry, nil).Handler()

	for _, test := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{"/", "text/html", "id=\"settings-list\""},
		{"/platform/app.js", "text/javascript", "renderPluginList"},
		{"/platform/plugins.mjs", "text/javascript", "renderPluginList"},
		{"/platform/settings.mjs", "text/javascript", "renderSettingsList"},
		{"/platform/loader.mjs", "text/javascript", "createMountLoader"},
		{"/platform/lite-agent.js", "text/javascript", "mountKey"},
		{"/bridge/bootstrap", "application/json", "\"version\":1"},
	} {
		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", test.path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, test.contentType) {
			t.Fatalf("%s content type = %q", test.path, got)
		}
		if !strings.Contains(rec.Body.String(), test.contains) {
			t.Fatalf("%s body does not contain %q", test.path, test.contains)
		}
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s missing nosniff", test.path)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/bridge/bootstrap", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var bootstrap Bootstrap
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrap); err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Mounts) != 1 || bootstrap.Mounts[0].Entry != "/plugins/demo/ui/main.js" {
		t.Fatalf("bootstrap = %#v", bootstrap)
	}
}

func TestHandlerServesOnlyPluginUIResources(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	handler := NewServer(registry, nil).Handler()

	req := httptest.NewRequest(http.MethodGet, "/plugins/demo/ui/theme.css", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("UI resource status = %d", rec.Code)
	}

	for _, path := range []string{
		"/plugins/demo/plugin.json",
		"/plugins/demo/ui/../outside.txt",
		"/plugins/missing/ui/main.js",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, body = %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestHandlerRejectsUnsupportedMethods(t *testing.T) {
	registry, _ := NewRegistry(plugin.Discoveries{})
	handler := NewServer(registry, nil).Handler()

	for _, path := range []string{"/", "/bridge/bootstrap", "/plugins/demo/ui/main.js"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
	}
}

func TestBootstrapDoesNotExposeLocalPaths(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	handler := NewServer(registry, nil).Handler()

	req := httptest.NewRequest(http.MethodGet, "/bridge/bootstrap", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), filepath.ToSlash(discovery.Dir)) ||
		strings.Contains(rec.Body.String(), filepath.ToSlash(t.TempDir())) {
		t.Fatalf("bootstrap leaks local paths: %s", rec.Body.String())
	}
}


