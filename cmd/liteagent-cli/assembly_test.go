package main_test

import (
	"os"
	"path/filepath"
	"testing"
)

// buildEchoPluginDir creates dir/<name>/ with a working echo plugin binary.
func buildEchoPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	echoBin := buildPkg(t, root, "./plugins/echo")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["echo"],
		"consumes": [],
		"entry": "`+name+`.exe"
	}`)
	copyFile(t, echoBin, dst)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}
