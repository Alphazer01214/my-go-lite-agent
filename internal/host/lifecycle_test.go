package host

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func writePluginPackage(t *testing.T, root, name string, manifest string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRescanPluginsFindsRuntimeDirectories(t *testing.T) {
	root := t.TempDir()
	writePluginPackage(t, root, "one", `{"name":"one","version":"1.0.0","protocol":1,"entry":"one"}`)
	h := newTestHost()
	h.pluginsDir = root

	found, err := h.RescanPlugins()
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Plugins) != 1 || found.Plugins[0].Manifest.Name != "one" {
		t.Fatalf("discoveries = %#v", found)
	}

	writePluginPackage(t, root, "two", `{"name":"two","version":"1.0.0","protocol":1,"entry":"two"}`)
	found, err = h.RescanPlugins()
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Plugins) != 2 {
		t.Fatalf("discoveries = %#v", found)
	}
}

func TestManualStopReturnsInactiveState(t *testing.T) {
	root := t.TempDir()
	writePluginPackage(t, root, "demo", `{"name":"demo","version":"1.0.0","protocol":1,"entry":"demo"}`)
	h := newTestHost()
	h.pluginsDir = root
	if _, err := h.RescanPlugins(); err != nil {
		t.Fatal(err)
	}

	if err := h.StopPlugin("demo"); err != nil {
		t.Fatal(err)
	}
	snapshot := h.PluginSnapshot()
	if len(snapshot) != 1 || snapshot[0].State != ProviderStateInactive {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestStartPluginWarnsButDoesNotBlockOnDependencies(t *testing.T) {
	root := t.TempDir()
	writePluginPackage(t, root, "base", `{"name":"base","version":"1.0.0","protocol":1,"entry":"base"}`)
	writePluginPackage(t, root, "dependent", `{"name":"dependent","version":"1.0.0","protocol":1,"entry":"dependent","depends_on":["base"]}`)
	h := newTestHost()
	h.pluginsDir = root
	if _, err := h.RescanPlugins(); err != nil {
		t.Fatal(err)
	}

	var logs []string
	h.hlog = func(message string) { logs = append(logs, message) }

	err := h.StartPlugin("dependent")
	if err != nil && strings.Contains(err.Error(), "dependency") {
		t.Fatalf("unhealthy dependency must not block start: %v", err)
	}

	var warned bool
	for _, line := range logs {
		if strings.Contains(line, "start warn:") && strings.Contains(line, "dependency base") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a start warning for the missing dependency, logs = %#v", logs)
	}
}

func TestLifecycleRejectsUnknownPlugin(t *testing.T) {
	h := newTestHost()
	h.discoveries = plugin.Discoveries{}
	if err := h.StartPlugin("missing"); !errors.Is(err, ErrPluginNotMounted) {
		t.Fatalf("start error = %v", err)
	}
	if err := h.StopPlugin("missing"); !errors.Is(err, ErrPluginNotMounted) {
		t.Fatalf("stop error = %v", err)
	}
}
