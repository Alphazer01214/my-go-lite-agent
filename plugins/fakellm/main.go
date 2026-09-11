// Command fakellm is a fixture LLM Plugin: streams evt chunks, then returns one assistant reply.
//
// Capability: llm
//   - complete: Call Payload {"messages":[{role,content}...]}; streams chunk evt; res {"content"}
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("llm", "complete", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		lastUser := ""
		for _, m := range in.Messages {
			if m.Role == "user" {
				lastUser = m.Content
			}
		}
		reply := "You said: " + lastUser

		// Stream deltas tagged with the request id so Host can attribute them to this Call.
		parts := []string{"You ", "said: "}
		if lastUser != "" {
			parts = append(parts, lastUser)
		}
		for _, p := range parts {
			payload, _ := json.Marshal(map[string]string{"delta": p})
			_ = s.EmitTo(req.ID, "llm", "chunk", payload)
		}
		return json.Marshal(map[string]string{"content": reply})
	})
	_ = s.Serve()
}
