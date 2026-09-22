package serve

import (
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func collectRelayEvents(s *Server, from string, f *protocol.Frame) []Event {
	var got []Event
	s.Subscribe(&Subscriber{OnEvent: func(e Event) { got = append(got, e) }})
	s.collectEvent(from, f)
	return got
}

// TestCollectEventRelaysGenericPluginEvts pins the ADR-0034 relay: id-less
// evts outside the presentation contract fan out on topic "evt" opaquely.
func TestCollectEventRelaysGenericPluginEvts(t *testing.T) {
	s := &Server{}
	got := collectRelayEvents(s, "session", &protocol.Frame{
		Type:    protocol.TypeEvt,
		Cap:     "choice",
		Method:  "ask",
		Payload: json.RawMessage(`{"id":"choice-1","kind":"tool_approval"}`),
	})
	if len(got) != 1 {
		t.Fatalf("want exactly one relayed event, got %d", len(got))
	}
	if got[0].Topic != "evt" {
		t.Fatalf("want topic evt, got %q", got[0].Topic)
	}
	m, ok := got[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("want map data, got %T", got[0].Data)
	}
	if m["cap"] != "choice" || m["method"] != "ask" {
		t.Fatalf("want cap/method passthrough, got %v", m)
	}
	payload, ok := m["payload"].(json.RawMessage)
	if !ok || string(payload) != `{"id":"choice-1","kind":"tool_approval"}` {
		t.Fatalf("want payload passthrough, got %v (%T)", m["payload"], m["payload"])
	}
}

// TestCollectEventDoesNotRelayIdTaggedEvts: id-tagged evts belong to an
// in-flight call, not the Medium fan-out.
func TestCollectEventDoesNotRelayIdTaggedEvts(t *testing.T) {
	s := &Server{}
	got := collectRelayEvents(s, "session", &protocol.Frame{
		Type: protocol.TypeEvt, ID: "fwd-1", Cap: "choice", Method: "ask",
	})
	if len(got) != 0 {
		t.Fatalf("id-tagged evt must stay call-scoped, got %d events", len(got))
	}
}

// TestCollectEventDoesNotDoubleRelayPresentation: presentation evts already
// have dedicated topics; the generic relay must not duplicate them.
func TestCollectEventDoesNotDoubleRelayPresentation(t *testing.T) {
	s := &Server{}
	got := collectRelayEvents(s, "agent", &protocol.Frame{
		Type:    protocol.TypeEvt,
		Cap:     PresentationCap,
		Method:  PresentationRenderMethod,
		Payload: json.RawMessage(`{"kind":"message_text","text":"hi"}`),
	})
	if len(got) != 1 || got[0].Topic != "presentation" {
		t.Fatalf("want single presentation topic event, got %+v", got)
	}
}
