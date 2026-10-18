package host

import (
	"testing"
	"time"
)

func TestCancelFrameForwardsAndPreservesPending(t *testing.T) {
	h := newTestHost("caller", "target")
	target := h.plugins["target"]
	target.state = ProviderStateActive
	target.gen = 2
	h.pending["fwd-1"] = &wait{
		kind: WaitPlugin, caller: "caller", callerPlugin: "caller",
		target: "target", targetPlugin: "target", targetGeneration: target.gen,
		frameID: "caller-1", capability: "demo", method: "run",
	}

	h.routeFrame("caller", 1, &Frame{
		Version: FrameVersion, ID: "caller-1", Type: FrameCancel,
		Capability: "demo", Method: "run", Reason: "cancelled",
	})

	frames := drainFrames(t, h, "target")
	if len(frames) != 1 || frames[0].Type != FrameCancel || frames[0].ID != "fwd-1" || frames[0].Reason != "cancelled" {
		t.Fatalf("frames = %#v", frames)
	}
	if _, ok := h.pending["fwd-1"]; !ok {
		t.Fatalf("pending = %#v", h.pending)
	}
}

func TestCancelFrameIgnoresOldGeneration(t *testing.T) {
	h := newTestHost("caller", "target")
	target := h.plugins["target"]
	target.state = ProviderStateActive
	target.gen = 2
	h.pending["fwd-1"] = &wait{
		kind: WaitPlugin, caller: "caller", callerPlugin: "caller",
		target: "target", targetPlugin: "target", targetGeneration: target.gen - 1,
		frameID: "caller-1", capability: "demo", method: "run",
	}

	h.routeFrame("caller", 1, &Frame{
		Version: FrameVersion, ID: "caller-1", Type: FrameCancel,
		Capability: "demo", Method: "run", Reason: "cancelled",
	})
	if frames := drainFrames(t, h, "target"); len(frames) != 0 {
		t.Fatalf("old generation received cancel: %#v", frames)
	}
}

func TestCancelTimeoutSynthesizesTerminalResponse(t *testing.T) {
	h := newTestHost("target")
	target := h.plugins["target"]
	target.state = ProviderStateActive
	h.cancelGrace = 10 * time.Millisecond
	response := make(chan *Frame, 1)
	h.pending["host-1"] = &wait{
		kind: WaitHost, caller: "webui", callerPlugin: "webui",
		target: "target", targetPlugin: "target", targetGeneration: target.gen,
		frameID: "host-1", capability: "demo", method: "run", resCh: response,
	}

	if err := h.cancelPending("host-1", "cancelled"); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-response:
		if frame.ErrorCode != CodeCallCancelled {
			t.Fatalf("response = %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel timeout did not synthesize response")
	}
}
