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
		{"missing name", Manifest{Version: "1", Protocol: 1, Entry: "x"}},
		{"missing version", Manifest{Name: "a", Protocol: 1, Entry: "x"}},
		{"bad protocol", Manifest{Name: "a", Version: "1", Protocol: 2, Entry: "x"}},
		{"missing entry", Manifest{Name: "a", Version: "1", Protocol: 1}},
		{"empty provides item", Manifest{Name: "a", Version: "1", Protocol: 1, Entry: "x", Provides: []string{""}}},
	}
	for _, tc := range cases {
		if err := tc.m.Validate(); err == nil {
			t.Errorf("%s: want error", tc.name)
		}
	}
}

func TestEntryExists(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Name: "a", Version: "1", Protocol: 1, Entry: "bin"}
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
