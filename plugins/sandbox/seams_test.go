package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestSeams(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	body := `{"consumes":[{"capability":"tool","method":"call"},{"capability":"shell","method":"execute"},{"capability":"","method":"x"}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	seams := loadManifestSeams(path)
	if len(seams) != 2 {
		t.Fatalf("seams = %+v", seams)
	}
	if seams[0].String() != "tool.call" || seams[1].String() != "shell.execute" {
		t.Fatalf("seams = %+v", seams)
	}
}

func TestLoadManifestSeamsFallsBack(t *testing.T) {
	seams := loadManifestSeams(filepath.Join(t.TempDir(), "missing.json"))
	if len(seams) != len(defaultSeams) {
		t.Fatalf("seams = %+v", seams)
	}
}
