// Command uidemo is a Web Medium Panel demo plugin (ADR-0009).
//
// Capability: none required. Presentation: mount-time Panel + ui.action echo.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	s := pluginsdk.New()
	// Mount-time sidebar Panel injection (also declared via plugin.json ui.entry).
	_ = s.EmitPanel(pluginsdk.PanelOp{
		Op:   "set",
		Slot: "sidebar",
		ID:   "mode-switch",
		HTML: `<div class="uidemo">
  <strong>Mode</strong>
  <div style="margin-top:6px;display:flex;gap:6px;flex-wrap:wrap">
    <button data-la-plugin="uidemo" data-la-panel="mode" data-la-event="set" data-la-value="chat">Chat</button>
    <button data-la-plugin="uidemo" data-la-panel="mode" data-la-event="set" data-la-value="agent">Agent</button>
  </div>
  <div id="uidemo-out" style="margin-top:8px;font-size:12px;color:#9aa0a6">—</div>
</div>`,
	})
	s.Handle("ui", "action", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Panel string          `json:"panel"`
			Event string          `json:"event"`
			Value json.RawMessage `json:"value"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		mode := string(in.Value)
		if len(mode) >= 2 && mode[0] == '"' {
			var m string
			_ = json.Unmarshal(in.Value, &m)
			mode = m
		}
		text := fmt.Sprintf("mode=%s event=%s", mode, in.Event)
		_ = s.EmitPanel(pluginsdk.PanelOp{
			Op:   "set",
			Slot: "toolbar-right",
			ID:   "mode-echo",
			HTML: `<div style="font-size:12px">` + text + `</div>`,
		})
		return json.Marshal(map[string]any{"ok": true, "text": text})
	})
	_ = s.Serve()
}
