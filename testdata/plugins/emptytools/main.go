// Command emptytools is a fixture Tools Plugin that registers no model-facing tools.
//
// Capability: tools
//   - list: empty tools array
//   - call: always errors
package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

func main() {
	s := pluginsdk.New()
	s.Handle("tools", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"tools": []any{}})
	})
	s.Handle("tools", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return nil, &protocol.FrameError{Code: "unknown_tool", Message: "emptytools has no tools"}
	})
	_ = s.Serve()
}
