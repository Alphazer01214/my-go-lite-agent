package host

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
)

type fakeUIPublisher struct {
	stateFrom      string
	statePayload   json.RawMessage
	commandFrom    string
	commandPayload json.RawMessage
	err            error
}

func (f *fakeUIPublisher) PublishState(from string, payload json.RawMessage) (json.RawMessage, error) {
	f.stateFrom = from
	f.statePayload = append(json.RawMessage(nil), payload...)
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`{}`), nil
}

func (f *fakeUIPublisher) PublishCommand(from string, payload json.RawMessage) (json.RawMessage, error) {
	f.commandFrom = from
	f.commandPayload = append(json.RawMessage(nil), payload...)
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`{}`), nil
}

func TestHostUIPublishUsesTrustedFrom(t *testing.T) {
	h := newTestHost("session")
	publisher := &fakeUIPublisher{}
	h.SetUIPublisher(publisher)

	for _, test := range []struct {
		method  string
		payload string
		check   func()
	}{
		{
			method:  HostMethodUIState,
			payload: `{"mount_id":"main","state":{"revision":1}}`,
			check: func() {
				if publisher.stateFrom != "session" || string(publisher.statePayload) != `{"mount_id":"main","state":{"revision":1}}` {
					t.Fatalf("state publish = from:%q payload:%s", publisher.stateFrom, publisher.statePayload)
				}
			},
		},
		{
			method:  HostMethodUICommand,
			payload: `{"mount_id":"main","command":"scroll","payload":{}}`,
			check: func() {
				if publisher.commandFrom != "session" || string(publisher.commandPayload) != `{"mount_id":"main","command":"scroll","payload":{}}` {
					t.Fatalf("command publish = from:%q payload:%s", publisher.commandFrom, publisher.commandPayload)
				}
			},
		},
	} {
		err := h.handleRequest("session", &Frame{
			ID: "session-ui-1", Type: FrameRequest,
			Capability: HostCapability, Method: test.method,
			Payload: json.RawMessage(test.payload),
		})
		if err != nil {
			t.Fatal(err)
		}
		frames := drainFrames(t, h, "session")
		if len(frames) == 0 || frames[len(frames)-1].ErrorCode != "" || string(frames[len(frames)-1].Payload) != "{}" {
			t.Fatalf("response = %#v", frames)
		}
		test.check()
	}
}

func TestHostUIPublishMapsPublisherErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{protocol.ErrUINotMounted, protocol.CodeUINotMounted},
		{protocol.ErrUIOwnerMismatch, protocol.CodeUIOwnerMismatch},
		{ErrBadPayload, CodeBadPayload},
		{errors.New("broken"), CodeHandlerError},
	} {
		h := newTestHost("session")
		h.SetUIPublisher(&fakeUIPublisher{err: test.err})
		err := h.handleRequest("session", &Frame{
			ID: "session-ui-1", Type: FrameRequest,
			Capability: HostCapability, Method: HostMethodUIState,
			Payload: json.RawMessage(`{}`),
		})
		if err == nil {
			t.Fatal("want publish error")
		}
		frames := drainFrames(t, h, "session")
		if len(frames) != 1 || frames[0].ErrorCode != test.code {
			t.Fatalf("response = %#v, want code %s", frames, test.code)
		}
	}
}

func TestHostUIPublishWithoutPublisher(t *testing.T) {
	h := newTestHost("session")
	err := h.handleRequest("session", &Frame{
		ID: "session-ui-1", Type: FrameRequest,
		Capability: HostCapability, Method: HostMethodUIState,
		Payload: json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("want missing publisher error")
	}
	frames := drainFrames(t, h, "session")
	if len(frames) != 1 || frames[0].ErrorCode != protocol.CodeUINotMounted {
		t.Fatalf("response = %#v", frames)
	}
}
