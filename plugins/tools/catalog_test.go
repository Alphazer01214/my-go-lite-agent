package main

import (
	"encoding/json"
	"testing"
)

func testTool(name, capability, method string) catalogTool {
	return catalogTool{
		Name:        name,
		Description: "test tool",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Capability:  capability,
		Method:      method,
	}
}

func TestBuildCatalogMergesAndSorts(t *testing.T) {
	built := buildCatalog(7, []infoResult{
		{tools: []catalogTool{testTool("shell_execute", "shell", "execute")}},
		{tools: []catalogTool{testTool("file_read", "file", "read")}},
	}, nil)

	if built.Generation != 7 {
		t.Fatalf("generation = %d, want 7", built.Generation)
	}
	if len(built.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(built.Tools))
	}
	if built.Tools[0].Name != "file_read" || built.Tools[1].Name != "shell_execute" {
		t.Fatalf("tools order = %v", []string{built.Tools[0].Name, built.Tools[1].Name})
	}
	if built.ByName["file_read"].Capability != "file" {
		t.Fatalf("byName missing file_read: %+v", built.ByName)
	}
}

func TestBuildCatalogDropsConflictsAndKeepsDiagnostics(t *testing.T) {
	built := buildCatalog(1, []infoResult{
		{tools: []catalogTool{testTool("read", "file", "read")}},
		{tools: []catalogTool{testTool("read", "git", "read")}},
		{diagnostic: &diagnostic{Capability: "shell", Code: "info_timeout", Message: "timeout"}},
	}, []diagnostic{{Code: "config_invalid", Message: "bad entry"}})

	if len(built.Tools) != 0 {
		t.Fatalf("tools = %+v, want none", built.Tools)
	}
	if len(built.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %+v, want 3", built.Diagnostics)
	}
	foundConflict := false
	for _, diagnostic := range built.Diagnostics {
		if diagnostic.Code == codeToolNameConflict {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("missing conflict diagnostic: %+v", built.Diagnostics)
	}
}
