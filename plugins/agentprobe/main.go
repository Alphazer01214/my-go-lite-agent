// Command agentprobe is a fixture Plugin: it calls Host agent/request through the star.
//
// Invoke payload:
//   - {} or omitted → agent/request with empty claim (rebuild from Session Log)
//   - {"cap":"smuggle"} → agent/request with unlogged messages (must be rejected by Host)
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("demo", "invoke", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var body struct {
			Cap string `json:"cap"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &body)
		}
		if body.Cap == "smuggle" {
			smuggle := json.RawMessage(`{"messages":[{"role":"user","content":"not-in-log"}]}`)
			return s.Call("agent", "request", smuggle)
		}
		return s.Call("agent", "request", json.RawMessage(`{}`))
	})
	_ = s.Serve()
}
