// Command echotool is a fixture Tools Plugin: lists one echo tool and executes it.
//
// Capability: tools
//   - list: → {"tools":[{name,description,input_schema}]}
//   - call: {"name","arguments"} → {"content", "additionalContexts":[{role,content}]}
//
// On success it also Emits a presentation.card evt (pure projection of args+result).
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// emitEchoCard is a pure Presentation Card projection: no I/O, clock, or randomness.
func emitEchoCard(s *pluginsdk.Server, argsText, result string) {
	data, _ := json.Marshal(map[string]string{
		"input":  argsText,
		"output": result,
	})
	payload, _ := json.Marshal(map[string]any{
		"cardType": "echo_result",
		"tool":     "echo_text",
		"data":     json.RawMessage(data),
	})
	_ = s.Emit("presentation", "card", payload)
}

func main() {
	s := pluginsdk.New()

	s.Handle("tools", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{
			"tools": []map[string]any{{
				"name":        "echo_text",
				"description": "Echo text back to the caller",
				"input_schema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"text": map[string]string{"type": "string"}},
					"required":   []string{"text"},
				},
			}},
		})
	})

	s.Handle("tools", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if in.Name != "echo_text" {
			return nil, &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
		}
		var args struct {
			Text string `json:"text"`
		}
		if len(in.Arguments) > 0 {
			_ = json.Unmarshal(in.Arguments, &args)
		}
		// Deterministic failure probe for main-seam tests.
		if args.Text == "boom" {
			return nil, &protocol.FrameError{Code: "tool_failed", Message: "echo_text refused boom"}
		}
		// Deterministic additionalContexts probe: tool result then extra model-visible note.
		if args.Text == "ctx" {
			emitEchoCard(s, args.Text, args.Text)
			return json.Marshal(map[string]any{
				"content": args.Text,
				"additionalContexts": []map[string]string{{
					"role":    "system",
					"content": "ADDITIONAL_CTX_MARKER",
				}},
			})
		}
		emitEchoCard(s, args.Text, args.Text)
		return json.Marshal(map[string]string{"content": args.Text})
	})

	_ = s.Serve()
}
