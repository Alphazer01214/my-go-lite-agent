package main_test

import (
	"os"
	"path/filepath"
	"testing"
)

// buildSessionPluginDir mirrors the CLI-side fixture (cmd/liteagent-cli).
func buildSessionPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/session")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["session"],
		"consumes": [],
		"entry": "`+name+`.exe",
		"commands": [
			{"name":"dump-trace","description":"Export Session Log facts as JSON","usage":"/`+name+` dump-trace [sessionId]"},
			{"name":"list","description":"List sessions","usage":"/`+name+` list"},
			{"name":"derive","description":"Print Model Context","usage":"/`+name+` derive [sessionId]"},
			{"name":"current","description":"Show Current Session id","usage":"/`+name+` current"}
		]
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// buildFakeLLMPluginDir mirrors the CLI-side fixture (cmd/liteagent-cli).
func buildFakeLLMPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/fakellm")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["llm"],
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
