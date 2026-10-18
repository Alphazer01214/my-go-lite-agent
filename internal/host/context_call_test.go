package host

import (
	"context"
	"testing"
	"time"
)

func TestCallWithEventsHonorsContextDeadline(t *testing.T) {
	h := newTestHost("webui", "target")
	h.routes["demo.run"] = "target"
	h.cancelGrace = 10 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	done := make(chan struct {
		frame *Frame
		err   error
	}, 1)
	go func() {
		frame, err := h.CallWithEvents(ctx, "webui", "demo", "run", mustJSON(map[string]any{}), nil)
		done <- struct {
			frame *Frame
			err   error
		}{frame: frame, err: err}
	}()

	req := waitForRequest(t, h, "target")
	if req.Capability != "demo" {
		t.Fatalf("request = %#v", req)
	}
	select {
	case result := <-done:
		if result.err != nil || result.frame == nil || result.frame.ErrorCode != CodeCallDeadlineExceeded {
			t.Fatalf("result = %#v, want call_deadline_exceeded", result)
		}
	case <-time.After(time.Second):
		t.Fatal("host call did not honor deadline")
	}
}
