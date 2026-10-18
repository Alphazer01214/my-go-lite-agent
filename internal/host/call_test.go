package host

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func waitForRequest(t *testing.T, h *Host, plugin string) *Frame {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if frames := drainFrames(t, h, plugin); len(frames) > 0 {
			return frames[len(frames)-1]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("plugin %s did not receive request", plugin)
	return nil
}

func TestCallWithEventsReturnsResponse(t *testing.T) {
	h := newTestHost("b")
	h.routes["llm.complete"] = "b"

	type result struct {
		frame *Frame
		err   error
	}
	done := make(chan result, 1)
	go func() {
		frame, err := h.CallWithEvents(context.Background(), "webui", "llm", "complete",
			mustJSON(map[string]string{"prompt": "hi"}), nil)
		done <- result{frame: frame, err: err}
	}()

	req := waitForRequest(t, h, "b")
	if !strings.HasPrefix(req.ID, HostCallIDPrefix) || req.From != "webui" {
		t.Fatalf("request = id:%s from:%s", req.ID, req.From)
	}
	if req.Type != FrameRequest || req.Capability != "llm" || req.Method != "complete" {
		t.Fatalf("request = %#v", req)
	}

	res := &Frame{
		Version: FrameVersion, ID: req.ID, Type: FrameResponse,
		Capability: "llm", Method: "complete",
		Payload: mustJSON(map[string]string{"answer": "ok"}),
	}
	if err := h.handleResponse(res); err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.frame.ID != req.ID || got.frame.From != "b" || got.frame.Type != FrameResponse {
		t.Fatalf("response = %#v", got.frame)
	}
	if len(h.pending) != 0 {
		t.Fatalf("pending = %#v", h.pending)
	}
}

func TestCallWithEventsPreservesEventOrder(t *testing.T) {
	h := newTestHost("b")
	h.routes["loop.turn"] = "b"

	events := make(chan *Frame, 3)
	done := make(chan *Frame, 1)
	go func() {
		frame, _ := h.CallWithEvents(context.Background(), "webui", "loop", "turn", mustJSON(map[string]string{}),
			func(frame *Frame) {
				events <- frame
			})
		done <- frame
	}()

	req := waitForRequest(t, h, "b")
	for _, method := range []string{"one", "two", "three"} {
		event := &Frame{
			Version: FrameVersion, ID: req.ID, Type: FrameEvent,
			Capability: "loop", Method: method,
			Payload: mustJSON(map[string]string{"sequence": method}),
		}
		if err := h.handleEvent("b", event); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"one", "two", "three"} {
		got := <-events
		if got.Method != want {
			t.Fatalf("event order = %q, want %q", got.Method, want)
		}
		if got.ID != req.ID || got.From != "b" {
			t.Fatalf("event = id:%s from:%s, want %s/b", got.ID, got.From, req.ID)
		}
		if string(got.Payload) != string(mustJSON(map[string]string{"sequence": want})) {
			t.Fatalf("event payload = %s, want sequence %s", got.Payload, want)
		}
	}

	if err := h.handleResponse(&Frame{
		Version: FrameVersion, ID: req.ID, Type: FrameResponse,
		Capability: "loop", Method: "turn",
	}); err != nil {
		t.Fatal(err)
	}
	if got := <-done; got == nil || got.Type != FrameResponse {
		t.Fatalf("response = %#v", got)
	}
}

func TestCallWithEventsRejectsInvalidAndHostCalls(t *testing.T) {
	h := newTestHost("b")
	h.routes["host.plugins"] = "b"

	for _, test := range []struct {
		name       string
		from       string
		capability string
		method     string
		want       error
	}{
		{name: "empty from", from: "", capability: "llm", method: "complete", want: ErrBadArguments},
		{name: "empty capability", from: "webui", capability: "", method: "complete", want: ErrBadArguments},
		{name: "empty method", from: "webui", capability: "llm", method: "", want: ErrBadArguments},
		{name: "host capability", from: "webui", capability: HostCapability, method: "plugins", want: ErrMethodNotFound},
		{name: "unknown route", from: "webui", capability: "ghost", method: "run", want: ErrMethodNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			frame, err := h.CallWithEvents(context.Background(), test.from, test.capability, test.method, mustJSON(map[string]string{}), nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if frame != nil {
				t.Fatalf("frame = %#v, want nil", frame)
			}
		})
	}

	if len(h.pending) != 0 {
		t.Fatalf("pending = %#v", h.pending)
	}
	if frames := drainFrames(t, h, "b"); len(frames) != 0 {
		t.Fatalf("b received frames: %#v", frames)
	}
}

func TestCallWithEventsReturnsPluginDownResponse(t *testing.T) {
	h := newTestHost("b")
	h.routes["llm.complete"] = "b"

	done := make(chan *Frame, 1)
	go func() {
		frame, err := h.CallWithEvents(context.Background(), "webui", "llm", "complete", mustJSON(map[string]string{}), nil)
		if err != nil {
			t.Errorf("CallWithEvents: %v", err)
		}
		done <- frame
	}()

	req := waitForRequest(t, h, "b")
	h.markUnhealthy("b", 1)

	select {
	case got := <-done:
		if got == nil || got.ErrorCode != CodePluginDown || got.ID != req.ID {
			t.Fatalf("response = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("CallWithEvents did not return after plugin death")
	}
}

func TestCallWithEventsAllowsNilEventCallback(t *testing.T) {
	h := newTestHost("b")
	h.routes["loop.turn"] = "b"

	done := make(chan *Frame, 1)
	go func() {
		frame, err := h.CallWithEvents(context.Background(), "webui", "loop", "turn", mustJSON(map[string]string{}), nil)
		if err != nil {
			t.Errorf("CallWithEvents: %v", err)
		}
		done <- frame
	}()

	req := waitForRequest(t, h, "b")
	if err := h.handleEvent("b", &Frame{
		Version: FrameVersion, ID: req.ID, Type: FrameEvent,
		Capability: "loop", Method: "chunk",
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.handleResponse(&Frame{
		Version: FrameVersion, ID: req.ID, Type: FrameResponse,
		Capability: "loop", Method: "turn",
	}); err != nil {
		t.Fatal(err)
	}
	if got := <-done; got == nil || got.Type != FrameResponse {
		t.Fatalf("response = %#v", got)
	}
}
