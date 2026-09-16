package main_test



import (

	"os"

	"os/exec"

	"path/filepath"

	"strings"

	"testing"

)



func buildEmptyToolsPluginDir(t *testing.T, root, pluginsDir, name string) {

	t.Helper()

	bin := buildPkg(t, root, "./plugins/emptytools")

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



// TestSubagentSyncToolResult: parent turn uses run_subagent tool; child Session is

// independent; parent tool_result contains the child final assistant text.

func TestSubagentSyncToolResult(t *testing.T) {

	root := moduleRoot(t)

	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")



	pluginsDir := t.TempDir()

	buildSessionPluginDir(t, root, pluginsDir, "session")

	buildStubLLMPluginDir(t, root, pluginsDir, "stubllm")

	buildEmptyToolsPluginDir(t, root, pluginsDir, "emptytools")



	buildAgentPluginDir(t, root, pluginsDir, "agent")

	cfg := filepath.Join(t.TempDir(), "assembly.json")

	writeFile(t, cfg, `{"plugins":["session","stubllm","emptytools","agent"]}`)



	// Fake LLM: first hop with tools →tool_call to first tool (run_subagent injected).

	// Input "SUBAGENT_TASK" becomes the subagent prompt.

	cmd := exec.Command(hostBin,

		"-plugins", pluginsDir,

		"-assembly", cfg,

		"-turn", "SUBAGENT_TASK",

		"-session-derive",

		"-session-query",

	)

	cmd.Env = hostEnv(t)

	out, err := cmd.CombinedOutput()

	if err != nil {

		t.Fatalf("subagent turn: %v\n%s", err, out)

	}

	s := string(out)

	if !strings.Contains(s, "turn ok") {

		t.Fatalf("want turn ok: %s", s)

	}

	// Parent must observe a run_subagent tool call.

	if !strings.Contains(s, "run_subagent") {

		t.Fatalf("want run_subagent tool call: %s", s)

	}

	// Child turn final assistant (fake-llm, no child tools) is returned as tool_result.

	if !strings.Contains(s, "You said: SUBAGENT_TASK") {

		t.Fatalf("want child assistant as parent tool_result content: %s", s)

	}

	// Parent must observe a tool role fact.

	if !strings.Contains(s, `"role":"tool"`) && !strings.Contains(s, `"role": "tool"`) {

		t.Fatalf("want tool role in parent derive: %s", s)

	}

	// Parent/child link: a subagent-*.jsonl exists with persisted session_meta.

	sessDir := ""

	for _, e := range cmd.Env {

		if strings.HasPrefix(e, "SESSION_DATA_DIR=") {

			sessDir = strings.TrimPrefix(e, "SESSION_DATA_DIR=")

		}

	}

	if sessDir == "" {

		t.Fatal("hostEnv must set SESSION_DATA_DIR")

	}

	entries, err := os.ReadDir(sessDir)

	if err != nil {

		t.Fatal(err)

	}

	foundChild := false

	for _, e := range entries {

		if strings.HasPrefix(e.Name(), "subagent-") && strings.HasSuffix(e.Name(), ".jsonl") {

			foundChild = true

			raw, err := os.ReadFile(filepath.Join(sessDir, e.Name()))

			if err != nil {

				t.Fatal(err)

			}

			if !strings.Contains(string(raw), `"type":"session_meta"`) && !strings.Contains(string(raw), `"type": "session_meta"`) {

				t.Fatalf("child session must persist session_meta parent link: %s", e.Name())

			}

			if !strings.Contains(string(raw), `"parentSession":"default"`) && !strings.Contains(string(raw), `"parentSession": "default"`) {

				t.Fatalf("child session parentSession must be default (empty parent →default): %s", e.Name())

			}

		}

	}

	if !foundChild {

		t.Fatalf("want a subagent-*.jsonl child session under %s, got %v", sessDir, entries)

	}

}



// TestSubagentAsyncRejected: mode=async is not supported in v1 (error lands as tool_result).

func TestSubagentAsyncRejected(t *testing.T) {

	root := moduleRoot(t)

	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")



	pluginsDir := t.TempDir()

	buildSessionPluginDir(t, root, pluginsDir, "session")

	buildEmptyToolsPluginDir(t, root, pluginsDir, "emptytools")

	buildAgentPluginDir(t, root, pluginsDir, "agent")

	bin := buildPkg(t, root, "./plugins/asyncsubllm")

	dst := filepath.Join(pluginsDir, "asyncsubllm", "asyncsubllm.exe")

	writeFile(t, filepath.Join(pluginsDir, "asyncsubllm", "plugin.json"), `{

		"name": "asyncsubllm",

		"version": "0.1.0",

		"protocol": 2,
		"autostart": true,

		"provides": ["llm"],

		"consumes": [],

		"entry": "asyncsubllm.exe"

	}`)

	b, err := os.ReadFile(bin)

	if err != nil {

		t.Fatal(err)

	}

	if err := os.WriteFile(dst, b, 0o755); err != nil {

		t.Fatal(err)

	}



	cfg := filepath.Join(t.TempDir(), "assembly.json")

	writeFile(t, cfg, `{"plugins":["session","asyncsubllm","emptytools","agent"]}`)



	cmd := exec.Command(hostBin,

		"-plugins", pluginsDir,

		"-assembly", cfg,

		"-turn", "try async",

		"-session-derive",

	)

	cmd.Env = hostEnv(t)

	out, err := cmd.CombinedOutput()

	// Tool errors are logged as tool_result and the turn still completes.

	if err != nil {

		t.Fatalf("async subagent turn: %v\n%s", err, out)

	}

	s := string(out)

	if !strings.Contains(s, "async subagent started") {

		t.Fatalf("want async subagent started in tool result: %s", s)

	}

}

