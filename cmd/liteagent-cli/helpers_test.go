package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

var binCache sync.Map // pkg -> built exe path (per test-binary process)

// buildPkg builds a package once per test-binary process and shares the exe
// across tests: the seam suite rebuilds the same host/plugin binaries dozens
// of times, which pushed the package past go's default 10m timeout.
func buildPkg(t *testing.T, root, pkg string) string {
	t.Helper()
	if v, ok := binCache.Load(pkg); ok {
		return v.(string)
	}
	out := filepath.Join(os.TempDir(), fmt.Sprintf("la-test-%d-%s", os.Getpid(), filepath.Base(pkg)+".exe"))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, b)
	}
	binCache.Store(pkg, out)
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// markAutostart sets autostart=true on a fixture plugin.json (ADR-0021 product path).
func markAutostart(t *testing.T, pluginsDir, name string) {
	t.Helper()
	path := filepath.Join(pluginsDir, name, "plugin.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Cheap inject: ensure the JSON object carries autostart true.
	s := string(raw)
	if !containsAutostart(s) {
		// insert after first {
		if i := indexByte(s, '{'); i >= 0 {
			s = s[:i+1] + "\n  \"autostart\": true," + s[i+1:]
		}
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsAutostart(s string) bool {
	return len(s) > 0 && (indexOf(s, `"autostart"`) >= 0)
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// hostEnv isolates the Session Plugin data dir per test (avoids shared ./sessions pollution).
func hostEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(), "SESSION_DATA_DIR="+t.TempDir())
}

// stubLLMMain is a deterministic test-only LLM: streams chunks, then one assistant reply.
// Materialized into a throwaway module at test time — no fixture package lives in the repo.
//
// Policy:
//   - tools provided and no tool-role message yet → one tool_call on tools[0]
//   - tool-role message present → "Tool said: <last tool>"
//   - else → "You said: <last user>"
const stubLLMMain = `package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type toolSchema struct {
	Name string ` + "`json:\"name\"`" + `
}

type toolCall struct {
	ID        string          ` + "`json:\"id\"`" + `
	Name      string          ` + "`json:\"name\"`" + `
	Arguments json.RawMessage ` + "`json:\"arguments\"`" + `
}

func main() {
	s := pluginsdk.New()
	s.Handle("llm", "complete", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Messages []struct {
				Role    string ` + "`json:\"role\"`" + `
				Content string ` + "`json:\"content\"`" + `
			} ` + "`json:\"messages\"`" + `
			Tools []toolSchema ` + "`json:\"tools\"`" + `
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}

		lastUser := ""
		lastTool := ""
		hasToolMsg := false
		for _, m := range in.Messages {
			switch m.Role {
			case "user":
				lastUser = m.Content
			case "tool":
				hasToolMsg = true
				lastTool = m.Content
			}
		}

		if len(in.Tools) > 0 && !hasToolMsg {
			args, _ := json.Marshal(map[string]string{"text": lastUser})
			out, _ := json.Marshal(map[string]any{
				"content": "",
				"tool_calls": []toolCall{{
					ID:        "call-1",
					Name:      in.Tools[0].Name,
					Arguments: args,
				}},
			})
			return out, nil
		}

		var reply string
		var parts []string
		if hasToolMsg {
			reply = "Tool said: " + lastTool
			parts = []string{"Tool ", "said: "}
			if lastTool != "" {
				parts = append(parts, lastTool)
			}
		} else {
			reply = "You said: " + lastUser
			parts = []string{"You ", "said: "}
			if lastUser != "" {
				parts = append(parts, lastUser)
			}
		}
		if !hasToolMsg {
			rp, _ := json.Marshal(map[string]string{
				"delta":   "thinking: " + lastUser,
				"channel": "reasoning",
			})
			_ = s.EmitTo(req.ID, "llm", "chunk", rp)
		}
		for _, p := range parts {
			payload, _ := json.Marshal(map[string]string{"delta": p, "channel": "content"})
			_ = s.EmitTo(req.ID, "llm", "chunk", payload)
		}
		return json.Marshal(map[string]any{"content": reply})
	})
	_ = s.Serve()
}
`

// buildStubLLMPluginDir compiles the in-test LLM stub into a plugin dir.
// Source is written to a temp module with a replace to the repo — nothing named
// stubllm/stubllm remains as a package.
func buildStubLLMPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildStubLLMBin(t, root)
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

func buildStubLLMBin(t *testing.T, root string) string {
	t.Helper()
	const cacheKey = "stub-llm"
	if v, ok := binCache.Load(cacheKey); ok {
		return v.(string)
	}
	src := filepath.Join(t.TempDir(), "stub-llm")
	writeFile(t, filepath.Join(src, "go.mod"),
		"module la-stub-llm\n\ngo 1.21\n\nrequire github.com/tomori/my-go-lite-agent v0.0.0\n\nreplace github.com/tomori/my-go-lite-agent => "+filepath.ToSlash(root)+"\n")
	writeFile(t, filepath.Join(src, "main.go"), stubLLMMain)
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = src
	if b, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy stub LLM: %v\n%s", err, b)
	}
	out := filepath.Join(os.TempDir(), fmt.Sprintf("la-test-%d-stub-llm.exe", os.Getpid()))
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = src
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build stub LLM: %v\n%s", err, b)
	}
	binCache.Store(cacheKey, out)
	return out
}
