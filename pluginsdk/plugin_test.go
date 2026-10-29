package pluginsdk

import (
	"context"
	"encoding/json"
	"testing"
)

type registerTestPlugin struct{}

func (registerTestPlugin) handle(
	_ context.Context,
	_ *Request,
) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true}`), nil
}

func TestRegisterAcceptsHandleFunc(t *testing.T) {
	p := NewPlugin("test")
	p.Register("example", "run", registerTestPlugin{}.handle)

	registrations := p.ListRegistered()
	if len(registrations) != 1 {
		t.Fatalf("registered handlers = %d, want 1", len(registrations))
	}
	if registrations[0].Capability != "example" || registrations[0].Method != "run" {
		t.Fatalf("registered route = %s.%s, want example.run", registrations[0].Capability, registrations[0].Method)
	}

	payload, err := registrations[0].Handler.handleFunc(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("handle returned error: %v", err)
	}
	if string(payload) != `{"ok":true}` {
		t.Fatalf("handle payload = %s, want {\"ok\":true}", payload)
	}
}

func TestRegisterRejectsNilHandleFunc(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Register did not panic for nil HandleFunc")
		}
	}()

	var handler HandleFunc
	NewPlugin("test").Register("example", "run", handler)
}
