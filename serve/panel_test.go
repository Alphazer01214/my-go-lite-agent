package serve

import (
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

func panelFrame(t *testing.T, op PanelOp) *protocol.Frame {
	t.Helper()
	raw, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	return &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeEvt,
		Cap:     PresentationCap,
		Method:  PresentationPanelMethod,
		Payload: raw,
	}
}

// collectPanelEvents runs one dispatchPanel and returns the events it fanned out.
func collectPanelEvents(s *Server, from string, f *protocol.Frame) (panels []PanelOp, status []string) {
	_ = s.Subscribe(&Subscriber{OnEvent: func(e Event) {
		switch ev := e.Data.(type) {
		case PanelOp:
			if e.Topic == "panel" {
				panels = append(panels, ev)
			}
		case map[string]string:
			if e.Topic == "status" {
				status = append(status, ev["status"])
			}
		}
	}})
	s.dispatchPanel(from, f)
	return panels, status
}

func TestDispatchPanelAcceptsComponentOps(t *testing.T) {
	s := &Server{}

	set := PanelOp{Op: "set", Slot: "sidebar", ID: "mode", Component: "uidemo-mode-panel", Props: json.RawMessage(`{"mode":"chat"}`)}
	panels, status := collectPanelEvents(s, "uidemo", panelFrame(t, set))
	if len(panels) != 1 || panels[0].Component != "uidemo-mode-panel" {
		t.Fatalf("want set op broadcast, got %+v (status %v)", panels, status)
	}
	if len(status) != 0 {
		t.Fatalf("want no warn, got %v", status)
	}

	clear := PanelOp{Op: "clear", Slot: "toolbar-right", ID: "mode"}
	panels, status = collectPanelEvents(s, "uidemo", panelFrame(t, clear))
	if len(panels) != 1 || panels[0].Op != "clear" {
		t.Fatalf("want clear op broadcast, got %+v (status %v)", panels, status)
	}
}

func TestDispatchPanelRejectsViolations(t *testing.T) {
	cases := []struct {
		name string
		from string
		op   PanelOp
	}{
		{"foreign prefix", "other", PanelOp{Op: "set", Slot: "sidebar", ID: "x", Component: "uidemo-mode-panel"}},
		{"missing component", "uidemo", PanelOp{Op: "set", Slot: "sidebar", ID: "x"}},
		{"append removed", "uidemo", PanelOp{Op: "append", Slot: "sidebar", ID: "x", Component: "uidemo-x"}},
		{"unknown slot", "uidemo", PanelOp{Op: "set", Slot: "footer", ID: "x", Component: "uidemo-x"}},
		{"bad tag case", "uidemo", PanelOp{Op: "set", Slot: "sidebar", ID: "x", Component: "Uidemo-X"}},
		{"tag without hyphen", "uidemo", PanelOp{Op: "set", Slot: "sidebar", ID: "x", Component: "uidemo"}},
		{"missing id", "uidemo", PanelOp{Op: "set", Slot: "sidebar", Component: "uidemo-x"}},
	}
	for _, tc := range cases {
		s := &Server{}
		panels, status := collectPanelEvents(s, tc.from, panelFrame(t, tc.op))
		if len(panels) != 0 {
			t.Errorf("%s: op must not broadcast, got %+v", tc.name, panels)
		}
		if len(status) != 1 || status[0] == "" {
			t.Errorf("%s: want one warn status, got %v", tc.name, status)
		}
	}
}

// Raw payloads cover shapes the typed PanelOp cannot express: a legacy html
// field (now dropped) and props that are not valid JSON.
func TestDispatchPanelRejectsRawPayloads(t *testing.T) {
	raws := map[string]string{
		"legacy html shape": `{"op":"set","slot":"sidebar","id":"x","html":"<b>hi</b>"}`,
		"malformed props":   `{"op":"set","slot":"sidebar","id":"x","component":"uidemo-x","props":{`,
	}
	for name, raw := range raws {
		s := &Server{}
		f := &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeEvt,
			Cap:     PresentationCap,
			Method:  PresentationPanelMethod,
			Payload: json.RawMessage(raw),
		}
		panels, status := collectPanelEvents(s, "uidemo", f)
		if len(panels) != 0 {
			t.Errorf("%s: op must not broadcast, got %+v", name, panels)
		}
		if len(status) != 1 || status[0] == "" {
			t.Errorf("%s: want one warn status, got %v", name, status)
		}
	}
}

func TestValidComponentTag(t *testing.T) {
	for _, ok := range []string{"uidemo-mode-panel", "a-b", "trace-view2"} {
		if !plugin.ValidComponentTag(ok) {
			t.Errorf("%q must be valid", ok)
		}
	}
	for _, bad := range []string{"", "ab", "-ab", "Ab-c", "a_b", "a--b!", "a-"} {
		if plugin.ValidComponentTag(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}
