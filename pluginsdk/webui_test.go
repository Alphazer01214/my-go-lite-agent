package pluginsdk

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestWebUIPublishesStateAndCommand(t *testing.T) {
	for _, test := range []struct {
		name        string
		call        func(*WebUI) error
		method      string
		wantPayload string
	}{
		{
			name: "state",
			call: func(ui *WebUI) error {
				return ui.SetState("main", json.RawMessage(`{"revision":2}`))
			},
			method:      "ui.state",
			wantPayload: `{"mount_id":"main","state":{"revision":2}}`,
		},
		{
			name: "command",
			call: func(ui *WebUI) error {
				return ui.SendCommand("main", "scroll", json.RawMessage(`{"position":"bottom"}`))
			},
			method:      "ui.command",
			wantPayload: `{"mount_id":"main","command":"scroll","payload":{"position":"bottom"}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, inW, outR := newPipePlugin(t)
			go p.Listen()

			done := make(chan error, 1)
			go func() { done <- test.call(p.WebUI()) }()

			req, err := protocol.ReadFrame(outR)
			if err != nil {
				t.Fatal(err)
			}
			if req.Type != protocol.FrameRequest || req.Capability != HostCapability || req.Method != test.method {
				t.Fatalf("request = %#v", req)
			}
			if string(req.Payload) != test.wantPayload {
				t.Fatalf("payload = %s, want %s", req.Payload, test.wantPayload)
			}

			if err := protocol.WriteFrame(inW, &protocol.Frame{
				Version: protocol.FrameVersion, ID: req.ID, Type: protocol.FrameResponse,
				Capability: req.Capability, Method: req.Method, Payload: json.RawMessage(`{}`),
			}); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("WebUI publish did not return")
			}
		})
	}
}

func TestWebUIRejectsMissingArguments(t *testing.T) {
	p, _, outR := newPipePlugin(t)
	ui := p.WebUI()

	for _, call := range []func() error{
		func() error { return ui.SetState("", json.RawMessage(`{}`)) },
		func() error { return ui.SendCommand("main", "", json.RawMessage(`{}`)) },
	} {
		err := call()
		if err == nil || Code(err) != CodeBadArguments {
			t.Fatalf("error = %v, want bad_arguments", err)
		}
	}

	readDone := make(chan error, 1)
	go func() {
		_, err := protocol.ReadFrame(outR)
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("invalid WebUI call wrote a request")
		}
	case <-time.After(50 * time.Millisecond):
	}
}
