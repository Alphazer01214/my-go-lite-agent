package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/serve"
)

type nopCommands struct{}

func (nopCommands) HandleOut(string) (string, bool, error) { return "", false, nil }
func (nopCommands) Complete(string) []string               { return nil }

func TestShellAndSDKServed(t *testing.T) {
	s := New(Options{Addr: "127.0.0.1:0", CommandPlane: nopCommands{}})
	ts := httptest.NewServer(s.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "id=\"split\"") || !strings.Contains(string(body), "id=\"chat-col\"") {
		t.Fatalf("want center split shell, got %d bytes", len(body))
	}
	if !strings.Contains(string(body), "new EventSource('/events')") {
		t.Fatal("shell must open live SSE")
	}

	res2, err := http.Get(ts.URL + "/sdk/lite-agent.js")
	if err != nil {
		t.Fatal(err)
	}
	js, _ := io.ReadAll(res2.Body)
	_ = res2.Body.Close()
	if !strings.Contains(string(js), "LiteAgent") {
		t.Fatal("sdk missing LiteAgent")
	}
}

func TestPluginUIAndTraversal(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uiDir, "main.js"), []byte("export{};"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := assembly.Plan{Mounted: []discovery.Found{{
		Dir: dir,
		Manifest: plugin.Manifest{
			Name: "demo", Version: "0.1.0", Protocol: plugin.CurrentProtocol, Entry: "x",
			UI: &plugin.UISpec{
				Entry: "main.js",
				Mounts: []plugin.UIMount{
					{Slot: "sidebar", Component: "demo-panel", Props: json.RawMessage(`{"a":1}`)},
				},
			},
		},
	}}}
	s := New(Options{Plan: plan, CommandPlane: nopCommands{}})
	ts := httptest.NewServer(s.http.Handler)
	defer ts.Close()

	// UI Entry module is served from the plugin's ui/ dir.
	ok, err := http.Get(ts.URL + "/plugin-ui/demo/main.js")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(ok.Body)
	_ = ok.Body.Close()
	if ok.StatusCode != http.StatusOK || !strings.Contains(string(b), "export") {
		t.Fatalf("want entry module ok, status=%d body=%s", ok.StatusCode, b)
	}

	// /api/plugins exposes the Panel Component contract for the Shell loader.
	res, err := http.Get(ts.URL + "/api/plugins")
	if err != nil {
		t.Fatal(err)
	}
	pb, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	for _, want := range []string{
		`"entry":"/plugin-ui/demo/main.js"`,
		`"component":"demo-panel"`,
		`"slot":"sidebar"`,
		`"name":"demo"`,
	} {
		if !strings.Contains(string(pb), want) {
			t.Fatalf("api/plugins missing %s: %s", want, pb)
		}
	}

	// Traversal must not serve secret.
	for _, p := range []string{
		"/plugin-ui/demo/../secret.txt",
		"/plugin-ui/demo/%2e%2e/secret.txt",
		"/plugin-ui/demo/..%2fsecret.txt",
	} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode == http.StatusOK && strings.Contains(string(raw), "nope") {
			t.Fatalf("traversal leaked via %s", p)
		}
	}
}

func TestReplayBuffer(t *testing.T) {
	s := New(Options{CommandPlane: nopCommands{}, ReplaySize: 10})
	s.broadcast(Event{Topic: "presentation", Data: serve.RenderIntent{Kind: "markdown_text", Text: "# hi"}})
	s.mu.Lock()
	n := len(s.replay)
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("want 1 replay event, got %d", n)
	}
}
