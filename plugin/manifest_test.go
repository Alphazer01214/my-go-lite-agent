package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	ok := Manifest{Name: "a", Version: "1.0.0", Protocol: CurrentProtocol, Entry: "a"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	cases := []struct {
		name string
		m    Manifest
	}{
		{"missing name", Manifest{Version: "1", Protocol: 2, Entry: "x"}},
		{"missing version", Manifest{Name: "a", Protocol: 2, Entry: "x"}},
		{"bad protocol", Manifest{Name: "a", Version: "1", Protocol: 3, Entry: "x"}},
		{"missing entry", Manifest{Name: "a", Version: "1", Protocol: 2}},
		{"empty provides item", Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "x", Provides: []string{""}}},
		{"bad name chars", Manifest{Name: "Help", Version: "1", Protocol: 2, Entry: "x"}},
	}
	for _, tc := range cases {
		if err := tc.m.Validate(); err == nil {
			t.Errorf("%s: want error", tc.name)
		}
	}
}

func TestEntryExists(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "bin"}
	if err := m.EntryExists(dir); err == nil {
		t.Fatal("want missing entry error")
	}
	if err := os.WriteFile(filepath.Join(dir, "bin"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.EntryExists(dir); err != nil {
		t.Fatalf("entry should exist: %v", err)
	}
}

func TestConflictsWithNativeCommand(t *testing.T) {
	if !(Manifest{Name: "help"}).ConflictsWithNativeCommand() {
		t.Fatal("help must conflict")
	}
	if !(Manifest{Name: "LP"}).ConflictsWithNativeCommand() {
		t.Fatal("LP must conflict case-insensitively")
	}
	if (Manifest{Name: "llm-openai"}).ConflictsWithNativeCommand() {
		t.Fatal("llm-openai must not conflict")
	}
}

func TestManifestCommandsValidate(t *testing.T) {
	ok := Manifest{
		Name: "llm-openai", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		Commands: []CommandSpec{{Name: "config", Description: "cfg", Usage: "/llm-openai config"}},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("commands rejected: %v", err)
	}
	bad := Manifest{
		Name: "p", Version: "1", Protocol: CurrentProtocol, Entry: "x",
		Commands: []CommandSpec{{Name: "has space"}},
	}
	if err := bad.Validate(); err == nil {
		t.Fatal("want whitespace command name error")
	}
}
