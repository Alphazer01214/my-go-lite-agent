package main_test

import (
	"os"
	"path/filepath"
	"testing"
)

func buildAgentPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/agent")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["loop", "agent-presets"],
		"hostFaces": ["config", "commands"],
		"consumes": [],
		"entry": "`+name+`.exe",
		"timeoutMs": 180000,
		"dependsOn": ["session", "llm-openai", "context-manager"]
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func buildSessionPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/session")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"autostart": true,
		"provides": ["session"],
		"hostFaces": ["config", "commands"],
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
