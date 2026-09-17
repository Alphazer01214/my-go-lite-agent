// Command consumer is a fixture Plugin: it provides demo and calls another
// Plugin only through Host point-named addressing (ADR-0030 Frame.to).
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("demo", "invoke", func(req *pluginsdk.Request) (json.RawMessage, error) {
		target := "echo"
		var body struct {
			Cap string `json:"cap"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &body)
			if body.Cap != "" {
				target = body.Cap
			}
		}
		return s.CallTo(target, "echo", "echo", json.RawMessage(`{"via":"host"}`))
	})
	_ = s.Serve()
}
