// Command agentprobe is a fixture Plugin: it calls Host agent/* through the star.
//
// Invoke payload.cap:
//   - omitted → agent/request with empty claim (rebuild from Session Log)
//   - "smuggle" → agent/request with unlogged messages (must be rejected by Host)
//   - "inject" → agent.inject one system note (must append, not wake a turn)
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
		switch body.Cap {
		case "smuggle":
			smuggle := json.RawMessage(`{"messages":[{"role":"user","content":"not-in-log"}]}`)
			return s.Call("agent", "request", smuggle)
		case "inject":
			inject := json.RawMessage(`{"role":"system","content":"PLUGIN_INJECTED"}`)
			return s.Call("agent", "inject", inject)
		default:
			return s.Call("agent", "request", json.RawMessage(`{}`))
		}
	})
	_ = s.Serve()
}
