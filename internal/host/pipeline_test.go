package host

import (
	"errors"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func pipelineProc(h *Host, id string, priority int, registrations ...RegistrationSnapshot) {
	p := h.plugins[id]
	p.instanceID = id
	p.pluginName = id
	p.state = ProviderStateActive
	p.discovery = plugin.Discovery{Manifest: plugin.Manifest{Name: id, Priority: priority}}
	p.registrations = registrations
}

func TestPipelineSelectsConsumersByPriority(t *testing.T) {
	h := newTestHost("high", "mid", "low")
	pipelineProc(h, "high", 200, RegistrationSnapshot{Capability: "tools", Method: "call"})
	pipelineProc(h, "mid", 100, RegistrationSnapshot{Capability: "tools", Method: "call"})
	pipelineProc(h, "low", 0, RegistrationSnapshot{Capability: "tools", Method: "call"})
	state := newPipelineState("", "tools", "call")

	first, err := h.selectPipelineCandidate(state)
	if err != nil || first.instanceID != "high" {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
	second, err := h.selectPipelineCandidate(state)
	if err != nil || second.instanceID != "mid" {
		t.Fatalf("second = %#v, err = %v", second, err)
	}
	third, err := h.selectPipelineCandidate(state)
	if err != nil || third.instanceID != "low" {
		t.Fatalf("third = %#v, err = %v", third, err)
	}
}

func TestNoActiveProviderError(t *testing.T) {
	h := newTestHost("provider")
	h.routes["llm.complete"] = "provider"
	h.plugins["provider"].state = ProviderStateInactive

	if err := h.noActiveProviderError("llm", "complete"); !errors.Is(err, ErrPluginNotReady) {
		t.Fatalf("inactive provider error = %v, want plugin_not_ready", err)
	}

	h.plugins["provider"].state = ProviderStateActive
	if err := h.noActiveProviderError("llm", "complete"); err != nil {
		t.Fatalf("active provider error = %v, want nil", err)
	}

	delete(h.routes, "llm.complete")
	if err := h.noActiveProviderError("llm", "complete"); !errors.Is(err, ErrPluginNotReady) {
		t.Fatalf("missing provider error = %v, want plugin_not_ready", err)
	}
}

func TestPipelineRejectsPriorityConflict(t *testing.T) {
	h := newTestHost("one", "two")
	pipelineProc(h, "one", 100, RegistrationSnapshot{Capability: "tools", Method: "call"})
	pipelineProc(h, "two", 100, RegistrationSnapshot{Capability: "tools", Method: "call"})
	if _, err := h.selectPipelineCandidate(newPipelineState("", "tools", "call")); err == nil {
		t.Fatal("same priority should conflict")
	}
}

func TestHostProceedForwardsToNextConsumer(t *testing.T) {
	h := newTestHost("current", "next", "other")
	pipelineProc(h, "current", 200, RegistrationSnapshot{Capability: "tools", Method: "call"})
	pipelineProc(h, "next", 100, RegistrationSnapshot{Capability: "tools", Method: "call"})
	state := newPipelineState("", "tools", "call")
	state.visited["current"] = true
	state.hops = 1
	state.nextPriority = 199
	h.pending["fwd-1"] = &wait{
		kind: WaitPlugin, caller: "original", callerPlugin: "original",
		target: "current", targetPlugin: "current", targetGeneration: 1,
		targetPriority: 200, pipeline: state, frameID: "original-1",
		capability: "tools", method: "call", payload: mustJSON(map[string]any{"n": 1}),
	}

	if err := h.hostProceed("current", &Frame{
		Version: FrameVersion, ID: "current-proceed", Type: FrameRequest,
		Capability: HostCapability, Method: HostMethodProceed,
		Payload: mustJSON(map[string]any{"call_id": "fwd-1", "payload": map[string]any{"n": 2}}),
	}); err != nil {
		t.Fatal(err)
	}

	var request *Frame
	for i := 0; i < 20; i++ {
		frames := drainFrames(t, h, "next")
		if len(frames) > 0 {
			request = frames[0]
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if request == nil || request.Priority != 100 {
		t.Fatalf("next request = %#v", request)
	}
	if err := h.handleRequest("other", &Frame{
		Version: FrameVersion, ID: "other-plugins", Type: FrameRequest,
		Capability: HostCapability, Method: HostMethodPlugins, Payload: mustJSON(map[string]any{}),
	}); err != nil {
		t.Fatal(err)
	}
	otherResponses := drainFrames(t, h, "other")
	if len(otherResponses) != 1 || otherResponses[0].Type != FrameResponse || otherResponses[0].ErrorCode != "" {
		t.Fatalf("other responses = %#v", otherResponses)
	}
	if err := h.handleEvent("next", &Frame{
		Version: FrameVersion, ID: request.ID, Type: FrameEvent,
		Capability: "tools", Method: "progress", Payload: mustJSON(map[string]any{"step": 1}),
	}); err != nil {
		t.Fatal(err)
	}
	events := drainFrames(t, h, "current")
	if len(events) != 1 || events[0].ID != "current-proceed" || events[0].Type != FrameEvent {
		t.Fatalf("events = %#v", events)
	}
	if err := h.handleResponse(&Frame{
		Version: FrameVersion, ID: request.ID, Type: FrameResponse,
		Capability: "tools", Method: "call", Payload: mustJSON(map[string]any{"ok": true}),
	}); err != nil {
		t.Fatal(err)
	}
	var responses []*Frame
	for i := 0; i < 20; i++ {
		responses = drainFrames(t, h, "current")
		if len(responses) >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(responses) != 2 || responses[0].Type != FrameEvent || responses[1].Type != FrameResponse || responses[1].ID != "current-proceed" || responses[1].ErrorCode != "" {
		t.Fatalf("responses = %#v", responses)
	}
}

func TestPipelineConsumersCanShareRouteKey(t *testing.T) {
	h := newTestHost("provider", "consumer")
	pipelineProc(h, "provider", 0, RegistrationSnapshot{Capability: "tools", Method: "call"})
	pipelineProc(h, "consumer", 100, RegistrationSnapshot{Capability: "tools", Method: "call"})
	h.plugins["consumer"].discovery.Manifest.Consumes = []plugin.ConsumesMethod{{Capability: "tools", Method: "call"}}

	for _, instanceID := range []string{"provider", "consumer"} {
		if err := h.handleRequest(instanceID, &Frame{
			ID: instanceID + "-reg", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
			Payload: mustJSON(map[string]any{"handlers": []map[string]string{{"capability": "tools", "method": "call"}}}),
		}); err != nil {
			t.Fatalf("register %s: %v", instanceID, err)
		}
	}

	state := newPipelineState("", "tools", "call")
	first, err := h.selectPipelineCandidate(state)
	if err != nil || first.instanceID != "consumer" {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
}

func TestPipelineRejectsLoopsAndHopLimits(t *testing.T) {
	h := newTestHost("one", "two")
	pipelineProc(h, "one", 200, RegistrationSnapshot{Capability: "tools", Method: "call"})
	pipelineProc(h, "two", 100, RegistrationSnapshot{Capability: "tools", Method: "call"})
	state := newPipelineState("", "tools", "call")
	state.visited["one"] = true
	state.visited["two"] = true
	if _, err := h.selectPipelineCandidate(state); err == nil {
		t.Fatal("pipeline loop should fail")
	}

	state = newPipelineState("", "tools", "call")
	state.hops = PipelineMaxHops
	if _, err := h.selectPipelineCandidate(state); err == nil {
		t.Fatal("hop limit should fail")
	}
}
