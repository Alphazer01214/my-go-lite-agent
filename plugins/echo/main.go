// Command echo is the reference Plugin: every req becomes a res with the same id.
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("echo", "echo", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return req.Payload, nil
	})
	_ = s.Serve()
}
