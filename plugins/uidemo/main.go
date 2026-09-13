// Command uidemo is the Web Panel Component reference plugin (ADR-0010).
//
// The plugin declares its UI statically via plugin.json (ui.entry + ui.mounts);
// interaction flows back as cap=ui, method=action, and the reply is a dynamic
// PanelOp that remounts a component in the Shell.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	s.Handle("ui", "action", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Panel string          `json:"panel"`
			Event string          `json:"event"`
			Value json.RawMessage `json:"value"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		var mode string
		if len(in.Value) > 0 {
			_ = json.Unmarshal(in.Value, &mode)
		}
		text := fmt.Sprintf("mode=%s event=%s", mode, in.Event)
		props, err := json.Marshal(map[string]string{"mode": mode, "event": in.Event})
		if err != nil {
			return nil, err
		}
		if err := s.EmitPanel(pluginsdk.PanelOp{
			Op:        "set",
			Slot:      "toolbar-right",
			ID:        "mode-echo",
			Component: "uidemo-echo-panel",
			Props:     props,
		}); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"ok": true, "text": text})
	})
	_ = s.Serve()
}
