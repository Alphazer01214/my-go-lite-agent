// Command sessionprobe is a fixture Plugin: it exercises multi-session session.* via the star.
//
// Invoke creates two sessions, appends distinct facts, derives both, and returns them.
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("demo", "invoke", func(req *pluginsdk.Request) (json.RawMessage, error) {
		for _, id := range []string{"alpha", "beta"} {
			if _, err := s.Call("session", "create", mustJSON(map[string]any{
				"sessionId": id,
			})); err != nil {
				return nil, err
			}
		}
		if _, err := s.Call("session", "append", mustJSON(map[string]any{
			"sessionId": "alpha",
			"type":      "message",
			"role":      "user",
			"content":   "alpha-only-marker",
		})); err != nil {
			return nil, err
		}
		if _, err := s.Call("session", "append", mustJSON(map[string]any{
			"sessionId": "beta",
			"type":      "message",
			"role":      "user",
			"content":   "beta-only-marker",
		})); err != nil {
			return nil, err
		}
		rawA, err := s.Call("session", "derive", mustJSON(map[string]any{"sessionId": "alpha"}))
		if err != nil {
			return nil, err
		}
		rawB, err := s.Call("session", "derive", mustJSON(map[string]any{"sessionId": "beta"}))
		if err != nil {
			return nil, err
		}
		var a, b struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.Unmarshal(rawA, &a)
		_ = json.Unmarshal(rawB, &b)
		return mustJSON(map[string]any{
			"alpha": a.Messages,
			"beta":  b.Messages,
		}), nil
	})
	_ = s.Serve()
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
