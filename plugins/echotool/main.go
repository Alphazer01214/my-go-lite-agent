// Command echotool is a fixture Tools Plugin: lists one echo tool and executes it.
//
// Capability: tools
//   - list: → {"tools":[{name,description,input_schema}]}
//   - call: {"name","arguments"} → {"content"}
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

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
		return json.Marshal(map[string]string{"content": args.Text})
	})

	_ = s.Serve()
}
