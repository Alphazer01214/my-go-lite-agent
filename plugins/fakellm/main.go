// Command fakellm is a fixture LLM Plugin: streams evt chunks, then returns one assistant reply.
//
// Capability: llm
//   - complete: Call Payload {"messages":[...], "tools":[...]}; streams chunk evt;
//     res {"content"} or {"content":"", "tool_calls":[...]}
//
// Tool policy (deterministic fixture):
//   - if tools are provided and no tool-role message exists yet → return one tool_call
//   - else if a tool-role message exists → final reply echoing that result
//   - else → "You said: <last user>"
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type toolSchema struct {
	Name string `json:"name"`
}

type toolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func main() {
	s := pluginsdk.New()
	s.Handle("llm", "complete", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []toolSchema `json:"tools"`
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

		// First model hop: tools available and none used yet → request one tool call.
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
		for _, p := range parts {
			payload, _ := json.Marshal(map[string]string{"delta": p})
			_ = s.EmitTo(req.ID, "llm", "chunk", payload)
		}
		return json.Marshal(map[string]any{"content": reply})
	})
	_ = s.Serve()
}
