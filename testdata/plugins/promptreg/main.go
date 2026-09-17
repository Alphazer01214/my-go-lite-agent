// Command promptreg is a fixture Plugin: it registers a Prompt Segment via the star.
//
// Invoke payload: {"text":"...","name":"...","order":N} → system-prompt.registerSegment
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("demo", "invoke", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var body struct {
			Name  string `json:"name"`
			Order int    `json:"order"`
			Text  string `json:"text"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &body)
		}
		if body.Name == "" {
			body.Name = "dynamic"
		}
		if body.Text == "" {
			body.Text = "DYNAMIC_SEGMENT_MARKER"
		}
		return s.CallTo("context-manager", "system-prompt", "registerSegment", mustJSON(map[string]any{
			"name":  body.Name,
			"order": body.Order,
			"text":  body.Text,
		}))
	})
	_ = s.Serve()
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
