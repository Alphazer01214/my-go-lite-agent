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
	if !strings.Contains(string(body), `id="split"`) || !strings.Contains(string(body), `id="chat-col"`) {
		t.Fatalf("want center split shell, got %d bytes", len(body))
	}
	// Shell face lives in ES modules under /app/ (ADR-0011 ticket 02).
	if !strings.Contains(string(body), `/app/main.js`) {
		t.Fatal("shell must load the module entry")
	}
	appRes, err := http.Get(ts.URL + "/app/events.js")
	if err != nil {
		t.Fatal(err)
	}
	appBody, _ := io.ReadAll(appRes.Body)
	_ = appRes.Body.Close()
	if appRes.StatusCode != http.StatusOK || !strings.Contains(string(appBody), "new EventSource('/events')") {
		t.Fatalf("shell must open live SSE via /app/events.js, status=%d", appRes.StatusCode)
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
	}, {
		// UI-only Plugin: no executable entry, Web UI only (ADR-0011).
		Dir: dir,
		Manifest: plugin.Manifest{
			Name: "webonly", Version: "0.1.0", Protocol: plugin.CurrentProtocol,
			UI: &plugin.UISpec{
				Entry: "main.js",
				Mounts: []plugin.UIMount{
					{Slot: "toolbar-right", Component: "webonly-panel"},
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
		`"name":"webonly"`,
		`"component":"webonly-panel"`,
		`"graph"`,
		`"edges"`,
		`"nodes"`,
	} {
		if !strings.Contains(string(pb), want) {
			t.Fatalf("api/plugins missing %s: %s", want, pb)
		}
	}
	// Dependency edge: a consumer of a provided Capability yields provider→consumer.
	// demo provides "demo" in this fixture; if anything consumes it the edge appears.
	var payload struct {
		Plugins []struct {
			Name     string   `json:"name"`
			Provides []string `json:"provides"`
			Consumes []string `json:"consumes"`
		} `json:"plugins"`
		Graph struct {
			Nodes []struct {
				ID    string   `json:"id"`
				Unmet []string `json:"unmet"`
			} `json:"nodes"`
			Edges []struct {
				From       string `json:"from"`
				To         string `json:"to"`
				Capability string `json:"capability"`
			} `json:"edges"`
		} `json:"graph"`
	}
	if err := json.Unmarshal(pb, &payload); err != nil {
		t.Fatalf("parse /api/plugins: %v\n%s", err, pb)
	}
	if len(payload.Graph.Nodes) < 2 {
		t.Fatalf("want graph nodes for mounted plugins, got %+v", payload.Graph.Nodes)
	}
	foundDemoNode := false
	for _, n := range payload.Graph.Nodes {
		if n.ID == "demo" {
			foundDemoNode = true
		}
	}
	if !foundDemoNode {
		t.Fatalf("graph nodes missing demo: %+v", payload.Graph.Nodes)
	}

	// Settings chrome module is served under /app/ (medium framework).
	mres, err := http.Get(ts.URL + "/app/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	mb, _ := io.ReadAll(mres.Body)
	_ = mres.Body.Close()
	if mres.StatusCode != http.StatusOK || !strings.Contains(string(mb), "openSettingsPanel") {
		t.Fatalf("want /app/settings.js, status=%d", mres.StatusCode)
	}

	// The UI-only plugin's UI Entry is served like any other.
	wo, err := http.Get(ts.URL + "/plugin-ui/webonly/main.js")
	if err != nil {
		t.Fatal(err)
	}
	wb, _ := io.ReadAll(wo.Body)
	_ = wo.Body.Close()
	if wo.StatusCode != http.StatusOK || !strings.Contains(string(wb), "export") {
		t.Fatalf("want ui-only entry module ok, status=%d body=%s", wo.StatusCode, wb)
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

func TestPluginDependencyGraph(t *testing.T) {
	plan := assembly.Plan{Mounted: []discovery.Found{
		{Manifest: plugin.Manifest{Name: "session", Version: "1", Protocol: plugin.CurrentProtocol,
			Provides: []string{"session"}}},
		{Manifest: plugin.Manifest{Name: "agentprobe", Version: "1", Protocol: plugin.CurrentProtocol,
			Provides: []string{"demo"}, Consumes: []string{"session"}}},
		{Manifest: plugin.Manifest{Name: "orphan", Version: "1", Protocol: plugin.CurrentProtocol,
			Consumes: []string{"missing-cap"}}},
	}}
	s := New(Options{Plan: plan, CommandPlane: nopCommands{}})
	ts := httptest.NewServer(s.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/plugins")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()

	var payload struct {
		Graph struct {
			Nodes []graphNode `json:"nodes"`
			Edges []graphEdge `json:"edges"`
		} `json:"graph"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}

	hasEdge := func(from, to, kind, capName string) bool {
		for _, e := range payload.Graph.Edges {
			if e.From == from && e.To == to && e.Kind == kind && e.Capability == capName {
				return true
			}
		}
		return false
	}
	// Bipartite: plugin provides capability. Host no longer draws host-uses
	// edges (ADR-0030: Host does not consume L1 capabilities).
	if !hasEdge("session", "cap:session", "provides", "session") {
		t.Fatalf("want session provides cap:session, edges=%+v", payload.Graph.Edges)
	}
	if hasEdge("cap:session", "host", "host-uses", "session") {
		t.Fatalf("host-uses edges must be gone under ADR-0030, edges=%+v", payload.Graph.Edges)
	}
	// Consumer edge: capability → plugin.
	if !hasEdge("cap:session", "agentprobe", "consumes", "session") {
		t.Fatalf("want cap:session consumed by agentprobe, edges=%+v", payload.Graph.Edges)
	}
	// Host node always present.
	foundHost := false
	var orphan *graphNode
	for i := range payload.Graph.Nodes {
		if payload.Graph.Nodes[i].ID == "host" && payload.Graph.Nodes[i].Kind == "host" {
			foundHost = true
		}
		if payload.Graph.Nodes[i].ID == "orphan" {
			orphan = &payload.Graph.Nodes[i]
		}
	}
	if !foundHost {
		t.Fatalf("missing host node: %+v", payload.Graph.Nodes)
	}
	if orphan == nil || len(orphan.Unmet) != 1 || orphan.Unmet[0] != "missing-cap" {
		t.Fatalf("orphan unmet = %+v", orphan)
	}
	// Even a provides-only plugin still yields edges (no empty graph).
	if len(payload.Graph.Edges) < 3 {
		t.Fatalf("want visible relationship edges, got %d: %+v", len(payload.Graph.Edges), payload.Graph.Edges)
	}
}

// TestPluginGraphShowsWholeCatalog pins the fix for "the dependency graph only
// appears after a conversation": under Autostart+dependsOn most plugins mount
// lazily (Agent Scheme → ensurePlugins), so the graph must be drawn from the
// whole Discovery catalog with per-plugin state, not from the live set alone.
func TestPluginGraphShowsWholeCatalog(t *testing.T) {
	dir := t.TempDir()
	writePlugin := func(name, manifest string) string {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "plugin.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		// Discovery validates that the declared entry exists on disk.
		if err := os.WriteFile(filepath.Join(d, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	coreDir := writePlugin("core", `{"name":"core","version":"1","protocol":3,"autostart":true,`+
		`"provides":["session"],"entry":"core"}`)
	writePlugin("filetools", `{"name":"filetools","version":"1","protocol":3,`+
		`"provides":["tools"],"entry":"filetools","dependsOn":["helper"]}`)
	writePlugin("helper", `{"name":"helper","version":"1","protocol":3,`+
		`"provides":["helper-cap"],"entry":"helper"}`)

	// Boot plan holds the autostart root only; the tools plugin is not mounted yet.
	plan := assembly.Plan{Mounted: []discovery.Found{{
		Dir: coreDir,
		Manifest: plugin.Manifest{Name: "core", Version: "1", Protocol: plugin.CurrentProtocol,
			Autostart: true, Provides: []string{"session"}, Entry: "core"},
	}}}
	s := New(Options{Plan: plan, PluginsDir: dir, CommandPlane: nopCommands{}})
	ts := httptest.NewServer(s.http.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/plugins")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()

	var payload struct {
		Graph struct {
			Nodes []graphNode `json:"nodes"`
			Edges []graphEdge `json:"edges"`
		} `json:"graph"`
		Summary struct {
			Discovered int `json:"discovered"`
			Mounted    int `json:"mounted"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if payload.Summary.Discovered < 3 || payload.Summary.Mounted != 1 {
		t.Fatalf("want discovered>=3 mounted=1, got %+v", payload.Summary)
	}
	nodes := map[string]graphNode{}
	for _, n := range payload.Graph.Nodes {
		nodes[n.ID] = n
	}
	if nodes["core"].State != StateMounted {
		t.Fatalf("core state = %q, want mounted", nodes["core"].State)
	}
	if nodes["filetools"].State != StateAvailable {
		t.Fatalf("filetools state = %q, want available (not yet ensured)", nodes["filetools"].State)
	}
	if !nodes["core"].Autostart {
		t.Fatal("core must be flagged autostart in the graph")
	}
	// dependsOn closure edge is visible before any Turn runs.
	foundDep := false
	for _, e := range payload.Graph.Edges {
		if e.From == "filetools" && e.To == "helper" && e.Kind == "depends-on" {
			foundDep = true
		}
	}
	if !foundDep {
		t.Fatalf("want filetools depends-on helper, edges=%+v", payload.Graph.Edges)
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
