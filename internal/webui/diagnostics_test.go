package webui

import (
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func diagnostic(pluginName, code string) plugin.Diagnostic {
	return plugin.Diagnostic{Plugin: pluginName, Code: code}
}

func TestCombineDiagnosticsKeepsRuntime(t *testing.T) {
	discovery := []plugin.Diagnostic{diagnostic("probe", "ui_entry_missing")}
	runtime := []plugin.Diagnostic{diagnostic("loop", "handler_error")}

	got := combineDiagnostics(discovery, runtime)
	if len(got) != 2 {
		t.Fatalf("got %d diagnostics, want 2", len(got))
	}
	if got[0].Plugin != "probe" || got[1].Plugin != "loop" {
		t.Fatalf("unexpected order: %#v", got)
	}

	nextDiscovery := []plugin.Diagnostic{diagnostic("other", "plugin_readme_missing")}
	got = combineDiagnostics(nextDiscovery, runtime)
	if len(got) != 2 || got[0].Plugin != "other" || got[1].Plugin != "loop" {
		t.Fatalf("runtime diagnostic lost after rescan: %#v", got)
	}
}

func TestCombineDiagnosticsCapsCombinedRing(t *testing.T) {
	discovery := make([]plugin.Diagnostic, 0, maxDiagnostics)
	runtime := make([]plugin.Diagnostic, 0, maxDiagnostics)
	for i := 0; i < maxDiagnostics; i++ {
		discovery = append(discovery, diagnostic("discovery", "d"))
		runtime = append(runtime, diagnostic("runtime", "r"))
	}

	got := combineDiagnostics(discovery, runtime)
	if len(got) != maxDiagnostics {
		t.Fatalf("got %d diagnostics, want %d", len(got), maxDiagnostics)
	}
	if got[len(got)-1].Plugin != "runtime" {
		t.Fatalf("newest runtime diagnostic was dropped: %#v", got[len(got)-1])
	}
}
