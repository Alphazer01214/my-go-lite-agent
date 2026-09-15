// Command project-context loads Workspace project instruction files.
//
// Capability: project-context
//   - load {workspace} → {text, source}
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const maxBytes = 32 * 1024

func loadFirst(workspace string, names []string) (string, string) {
	if workspace == "" {
		return "", ""
	}
	for _, name := range names {
		p := filepath.Join(workspace, name)
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		text := string(raw)
		if len(text) > maxBytes {
			text = text[:maxBytes] + "\n... (truncated)\n"
		}
		return text, name
	}
	return "", ""
}

func main() {
	s := pluginsdk.New()
	s.Handle("project-context", "load", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Workspace string `json:"workspace"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &in); err != nil {
				return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
			}
		}
		// Prefer AGENTS.md; fall back to CLAUDE.md (spec).
		text, source := loadFirst(in.Workspace, []string{"AGENTS.md", "CLAUDE.md"})
		text = strings.TrimSpace(text)
		return json.Marshal(map[string]any{
			"text":   text,
			"source": source,
		})
	})
	_ = s.Serve()
}
