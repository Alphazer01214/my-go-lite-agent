package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func filterEnv(env []string, drop ...string) []string {
	dropSet := make(map[string]bool, len(drop))
	for _, k := range drop {
		dropSet[k] = true
	}
	var out []string
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if dropSet[k] {
			continue
		}
		out = append(out, e)
	}
	return out
}

func buildLLMOpenAIPluginDir(t *testing.T, root, pluginsDir, name, baseURL, model string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/llm-openai")
	dir := filepath.Join(pluginsDir, name)
	dst := filepath.Join(dir, name+".exe")
	writeFile(t, filepath.Join(dir, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["llm"],
		"consumes": [],
		"entry": "`+name+`.exe",
		"timeoutMs": 60000,
		"description": "OpenAI-compatible LLM provider",
		"commands": [{"name":"config","description":"Show or set API key / model / baseURL","usage":"/`+name+` config [get|set key=value]"}]
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]string{
		"baseURL": baseURL,
		"apiKey":  "test-key",
		"model":   model,
	}
	raw, _ := json.Marshal(cfg)
	writeFile(t, filepath.Join(dir, "config.json"), string(raw))
}

// fakeOpenAI serves a minimal OpenAI-compatible chat/completions stream.
// When wantTools is true, it asserts the request included a tools array.
func fakeOpenAI(t *testing.T, reply string, wantTools bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if wantTools && len(body.Tools) == 0 {
			http.Error(w, `{"error":{"message":"expected tools in request"}}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		half := len(reply) / 2
		if half < 1 {
			half = len(reply)
		}
		parts := []string{reply[:half], reply[half:]}
		for _, p := range parts {
			if p == "" {
				continue
			}
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]string{"content": p},
				}},
			})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flusher != nil {
				flusher.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})
	return httptest.NewServer(mux)
}

func TestLLMOpenAIRealTurn(t *testing.T) {
	srv := fakeOpenAI(t, "Hello from DeepSeek-compatible API", false)
	defer srv.Close()

	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildLLMOpenAIPluginDir(t, root, pluginsDir, "llm-openai", srv.URL, "deepseek-flash")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","llm-openai","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hi",
		"-session-derive",
	)
	cmd.Env = hostEnv(t)
	// Ensure env does not override fixture config.
	cmd.Env = append(os.Environ(), "OPENAI_API_KEY=", "OPENAI_BASE_URL=", "OPENAI_MODEL=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("llm-openai turn: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "turn ok") {
		t.Fatalf("want turn ok: %s", s)
	}
	if !strings.Contains(s, "Hello from DeepSeek-compatible API") {
		t.Fatalf("want streamed reply in output: %s", s)
	}
}

// TestLLMOpenAIForwardsTools: Host tools schema must reach the OpenAI request body.
func TestLLMOpenAIForwardsTools(t *testing.T) {
	srv := fakeOpenAI(t, "ok-with-tools", true)
	defer srv.Close()

	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildLLMOpenAIPluginDir(t, root, pluginsDir, "llm-openai", srv.URL, "deepseek-flash")
	buildEchoToolPluginDir(t, root, pluginsDir, "echotool")

	buildAgentPluginDir(t, root, pluginsDir, "agent")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","llm-openai","echotool","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "use tools",
	)
	cmd.Env = filterEnv(os.Environ(), "OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tools forward turn: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "turn ok") {
		t.Fatalf("want turn ok: %s", out)
	}
}

func TestLLMOpenAIMissingKey(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildLLMOpenAIPluginDir(t, root, pluginsDir, "llm-openai", "http://127.0.0.1:1", "deepseek-flash")
	// Remove apiKey from config by rewriting empty key.
	buildAgentPluginDir(t, root, pluginsDir, "agent")
	writeFile(t, filepath.Join(pluginsDir, "llm-openai", "config.json"),
		`{"baseURL":"http://127.0.0.1:1","apiKey":"","model":"deepseek-flash"}`)

	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","llm-openai","agent"]}`)

	cmd := exec.Command(hostBin,
		"-plugins", pluginsDir,
		"-assembly", cfg,
		"-turn", "hi",
	)
	cmd.Env = append(hostEnv(t), "OPENAI_API_KEY=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want missing key failure: %s", out)
	}
	if !strings.Contains(string(out), "api_key") && !strings.Contains(string(out), "OPENAI_API_KEY") {
		t.Fatalf("want missing key diagnosis: %s", out)
	}
}
