// Command consumer is a fixture Plugin: it provides demo and calls another Capability only through Host.
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("demo", "invoke", func(req *pluginsdk.Request) (json.RawMessage, error) {
		capName := "echo"
		var body struct {
			Cap string `json:"cap"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &body)
			if body.Cap != "" {
				capName = body.Cap
			}
		}
		return s.Call(capName, "echo", json.RawMessage(`{"via":"host"}`))
	})
	_ = s.Serve()
}
