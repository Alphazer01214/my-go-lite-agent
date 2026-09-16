package main_test

import (
	"os"
	"path/filepath"
	"testing"
)

func buildFileToolsPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/filetools")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["tools"],
		"consumes": [],
		"entry": "`+name+`.exe"
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}
