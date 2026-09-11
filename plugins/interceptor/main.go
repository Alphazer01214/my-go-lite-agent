// Command interceptor is a fixture Interceptor Plugin for the dual Waterfall.
//
// Capability: interceptor
//   - before: {"from","cap","method","payload"} → {"action":"allow|rewrite|reject", "payload"?, "reason"?}
//
// Mode is read from mode.allow|rewrite|reject next to the executable (default allow).
//   - allow  → action allow
//   - rewrite → action rewrite with payload {"intercepted":true}
//   - reject → action reject
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func mode() string {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "mode.txt"))
	if err != nil {
		return "allow"
	}
	return strings.TrimSpace(string(b))
}

func main() {
	s := pluginsdk.New()
	s.Handle("interceptor", "before", func(req *pluginsdk.Request) (json.RawMessage, error) {
		switch mode() {
		case "reject":
			return json.Marshal(map[string]string{
				"action": "reject",
				"reason": "policy_reject",
			})
		case "rewrite":
			return json.Marshal(map[string]any{
				"action":  "rewrite",
				"payload": json.RawMessage(`{"intercepted":true}`),
				"reason":  "policy_rewrite",
			})
		default:
			return json.Marshal(map[string]string{"action": "allow"})
		}
	})
	_ = s.Serve()
}
