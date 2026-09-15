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
// across tests (mirrors cmd/liteagent-cli's helper).
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

// hostEnv isolates the Session Plugin data dir per test (avoids shared ./sessions pollution).
func hostEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(), "SESSION_DATA_DIR="+t.TempDir())
}

func buildAgentPluginDir(t *testing.T, root, pluginsDir, name string) {
	t.Helper()
	bin := buildPkg(t, root, "./plugins/agent")
	dst := filepath.Join(pluginsDir, name, name+".exe")
	writeFile(t, filepath.Join(pluginsDir, name, "plugin.json"), `{
		"name": "`+name+`",
		"version": "0.1.0",
		"protocol": 2,
		"provides": ["loop"],
		"consumes": [],
		"entry": "`+name+`.exe",
		"timeoutMs": 180000
	}`)
	b, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeTestLayout writes a minimal base layout for the Web Medium (ADR-0012).
func writeTestLayout(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layout.json")
	writeFile(t, path, `{
	  "pages": [
	    {"slug":"main","title":"Chat","path":"/","slots":[
	      {"id":"sidebar","role":"session-rail","preferred":"session-rail"},
	      {"id":"chat","role":"session-view","preferred":"session-view"},
	      {"id":"trace","role":"session-trace"},
	      {"id":"toolbar-right","role":"panel"},
	      {"id":"main-overlay","role":"overlay"}
	    ]},
	    {"slug":"trace","title":"Trace","path":"/trace","slots":[
	      {"id":"main","role":"session-trace"}
	    ]}
	  ]
	}`)
	return path
}

// stubLLMMain / buildStubLLMBin: deterministic test LLM built from a temp module
// (mirrors cmd/liteagent-cli). No fixture package lives in the repo.
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
