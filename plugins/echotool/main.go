// Command echotool is a fixture Tools Plugin: lists one echo tool and executes it.
//
// Capability: tools
//   - list: → {"tools":[{name,description,input_schema}]}
//   - call: {"name","arguments"} → {"content", "additionalContexts":[{role,content}]}
//
// On success it also Emits a presentation.card evt built by a pure projection of args+result.
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// echoCard is a pure Presentation Card projection: no I/O, clock, or randomness.
func echoCard(argsText, result string) pluginsdk.Card {
	data, _ := json.Marshal(map[string]string{
		"input":  argsText,
		"output": result,
	})
	return pluginsdk.Card{
		CardType: "echo_result",
		Tool:     "echo_text",
		Data:     data,
	}
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

		result := args.Text
		var body any = map[string]string{"content": result}
		// Deterministic additionalContexts probe: tool result then extra model-visible note.
		if args.Text == "ctx" {
			body = map[string]any{
				"content": result,
				"additionalContexts": []map[string]string{{
					"role":    "system",
					"content": "ADDITIONAL_CTX_MARKER",
				}},
			}
		}

		// Transport is separate from the pure projection.
		_ = s.EmitCard(echoCard(args.Text, result))
		return json.Marshal(body)
	})

	_ = s.Serve()
}
