package host

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func configureFakePlugin(h *Host, instanceID, pluginName string) {
	p := h.plugins[instanceID]
	if p == nil {
		return
	}
	p.instanceID = instanceID
	p.pluginName = pluginName
}

func TestExecutionContextInheritsCapabilityBinding(t *testing.T) {
	h := newTestHost("llm", "llm-new", "agent")
	configureFakePlugin(h, "llm", "llm")
	configureFakePlugin(h, "llm-new", "llm")
	configureFakePlugin(h, "agent", "agent")
	h.provides["llm"] = "llm"
	h.routes["llm.complete"] = "llm"

	root, err := h.CreateExecutionContext("agent", "", map[string]string{
		"llm": "llm-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.RootID != root.ID || root.Bindings["llm"] != "llm-new" {
		t.Fatalf("root = %#v", root)
	}

	child, err := h.CreateExecutionContext("agent", root.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if child.RootID != root.ID || child.ParentID != root.ID {
		t.Fatalf("child = %#v", child)
	}
	if child.Bindings["llm"] != "llm-new" {
		t.Fatalf("child bindings = %#v", child.Bindings)
	}
}

func TestCallWithEventsInContextUsesBoundInstance(t *testing.T) {
	h := newTestHost("llm", "llm-new", "agent")
	configureFakePlugin(h, "llm", "llm")
	configureFakePlugin(h, "llm-new", "llm")
	configureFakePlugin(h, "agent", "agent")
	h.provides["llm"] = "llm"
	h.routes["llm.complete"] = "llm"

	context, err := h.CreateExecutionContext("agent", "", map[string]string{
		"llm": "llm-new",
	})
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		frame *Frame
		err   error
	}
	done := make(chan result, 1)
	go func() {
		frame, err := h.CallWithEventsInContext(context.ID, "agent", "llm", "complete",
			mustJSON(map[string]string{"prompt": "hi"}), nil)
		done <- result{frame: frame, err: err}
	}()

	req := waitForRequest(t, h, "llm-new")
	if req.ContextID != context.ID {
		t.Fatalf("request context = %q, want %q", req.ContextID, context.ID)
	}
	if frames := drainFrames(t, h, "llm"); len(frames) != 0 {
		t.Fatalf("default instance received %#v", frames)
	}
	if err := h.handleResponse(&Frame{
		Version: FrameVersion, ID: req.ID, Type: FrameResponse,
		Capability: "llm", Method: "complete", ContextID: context.ID,
	}); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.frame.From != "llm" || got.frame.ContextID != context.ID {
		t.Fatalf("response = %#v", got.frame)
	}
}

func TestCloseExecutionContextFailsPendingCall(t *testing.T) {
	h := newTestHost("llm", "agent")
	configureFakePlugin(h, "llm", "llm")
	configureFakePlugin(h, "agent", "agent")
	h.provides["llm"] = "llm"
	h.routes["llm.complete"] = "llm"

	context, err := h.CreateExecutionContext("agent", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan *Frame, 1)
	go func() {
		frame, _ := h.CallWithEventsInContext(context.ID, "agent", "llm", "complete", mustJSON(map[string]string{}), nil)
		done <- frame
	}()
	req := waitForRequest(t, h, "llm")
	if req.ContextID != context.ID {
		t.Fatalf("request context = %q", req.ContextID)
	}

	if err := h.CloseExecutionContext("agent", context.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.ErrorCode != CodeContextClosed || got.ID != req.ID {
			t.Fatalf("response = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("pending call did not fail")
	}
}

func TestExecutionContextRejectsForeignParticipant(t *testing.T) {
	h := newTestHost("agent", "other")
	configureFakePlugin(h, "agent", "agent")
	configureFakePlugin(h, "other", "other")
	context, err := h.CreateExecutionContext("agent", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = h.handleRequest("other", &Frame{
		ID: "other-1", Type: FrameRequest, ContextID: context.ID,
		Capability: "demo", Method: "run", Payload: mustJSON(map[string]string{}),
	})
	if !errors.Is(err, ErrContextParticipant) {
		t.Fatalf("error = %v, want participant mismatch", err)
	}
}

func TestInstanceSnapshotTracksGenerationAndConfigRevision(t *testing.T) {
	h := newTestHost("llm")
	configureFakePlugin(h, "llm", "llm")
	h.plugins["llm"].configRevision = "cfg-9"
	h.plugins["llm"].gen = 4

	got := h.InstanceSnapshot()
	if len(got) != 1 || got[0].InstanceID != "llm" || got[0].PluginName != "llm" {
		t.Fatalf("snapshot = %#v", got)
	}
	if got[0].ConfigRevision != "cfg-9" || got[0].Generation != 4 || got[0].State != PluginInstanceStateMounted {
		t.Fatalf("snapshot = %#v", got[0])
	}
}

func TestHostContextMethodsManageContext(t *testing.T) {
	h := newTestHost("llm", "agent")
	configureFakePlugin(h, "llm", "llm")
	configureFakePlugin(h, "agent", "agent")
	h.provides["llm"] = "llm"

	create := &Frame{
		ID: "agent-1", Type: FrameRequest, Capability: HostCapability,
		Method: HostMethodContextCreate,
		Payload: mustJSON(map[string]any{
			"bindings": map[string]string{"llm": "llm"},
		}),
	}
	if err := h.handleRequest("agent", create); err != nil {
		t.Fatal(err)
	}
	responses := drainFrames(t, h, "agent")
	if len(responses) != 1 || responses[0].ErrorCode != "" {
		t.Fatalf("create responses = %#v", responses)
	}
	var created struct {
		Context ExecutionContext `json:"context"`
	}
	if err := json.Unmarshal(responses[0].Payload, &created); err != nil {
		t.Fatal(err)
	}
	if created.Context.ID == "" {
		t.Fatalf("created = %#v", created)
	}

	if err := h.handleRequest("agent", &Frame{
		ID: "agent-3", Type: FrameRequest, Capability: HostCapability,
		Method:  HostMethodContextClose,
		Payload: mustJSON(map[string]string{"context_id": created.Context.ID}),
	}); err != nil {
		t.Fatal(err)
	}
	got := drainFrames(t, h, "agent")
	if len(got) == 0 || got[len(got)-1].ErrorCode != "" {
		t.Fatalf("close responses = %#v", got)
	}
}

func TestHostInstanceCreateReportsUnknownPlugin(t *testing.T) {
	h := newTestHost("agent")
	configureFakePlugin(h, "agent", "agent")
	err := h.handleRequest("agent", &Frame{
		ID: "agent-1", Type: FrameRequest, Capability: HostCapability,
		Method:  HostMethodInstanceCreate,
		Payload: mustJSON(map[string]string{"plugin_name": "missing"}),
	})
	if !errors.Is(err, ErrPluginNotMounted) {
		t.Fatalf("error = %v, want plugin not mounted", err)
	}
	got := drainFrames(t, h, "agent")
	if len(got) == 0 || got[len(got)-1].ErrorCode != CodePluginNotMounted {
		t.Fatalf("responses = %#v", got)
	}
}

func TestForwardAddsTargetToExecutionContext(t *testing.T) {
	h := newTestHost("llm", "session", "agent")
	configureFakePlugin(h, "llm", "llm")
	configureFakePlugin(h, "session", "session")
	configureFakePlugin(h, "agent", "agent")
	h.provides["llm"] = "llm"
	h.provides["session"] = "session"
	h.routes["llm.complete"] = "llm"
	h.routes["session.save"] = "session"

	context, err := h.CreateExecutionContext("agent", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.handleRequest("agent", &Frame{
		ID: "agent-1", ContextID: context.ID, Type: FrameRequest,
		Capability: "llm", Method: "complete", Payload: mustJSON(map[string]string{}),
	}); err != nil {
		t.Fatal(err)
	}

	if err := h.handleRequest("llm", &Frame{
		ID: "llm-1", ContextID: context.ID, Type: FrameRequest,
		Capability: "session", Method: "save", Payload: mustJSON(map[string]string{}),
	}); err != nil {
		t.Fatalf("target participant could not propagate context: %v", err)
	}
}

func TestRouteFrameRejectsStaleGeneration(t *testing.T) {
	h := newTestHost("demo")
	configureFakePlugin(h, "demo", "demo")

	h.routeFrame("demo", 2, &Frame{
		ID: "stale-1", Type: FrameRequest, Capability: "demo", Method: "run",
		Payload: mustJSON(map[string]string{}),
	})
	if got := drainFrames(t, h, "demo"); len(got) != 0 {
		t.Fatalf("stale frame was routed: %#v", got)
	}
}

func TestCloseExecutionContextClosesDescendants(t *testing.T) {
	h := newTestHost("llm", "agent")
	configureFakePlugin(h, "llm", "llm")
	configureFakePlugin(h, "agent", "agent")
	h.provides["llm"] = "llm"
	h.routes["llm.complete"] = "llm"

	root, err := h.CreateExecutionContext("agent", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := h.CreateExecutionContext("agent", root.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan *Frame, 1)
	go func() {
		frame, _ := h.CallWithEventsInContext(child.ID, "agent", "llm", "complete", mustJSON(map[string]string{}), nil)
		done <- frame
	}()
	req := waitForRequest(t, h, "llm")
	if req.ContextID != child.ID {
		t.Fatalf("request context = %q", req.ContextID)
	}

	if err := h.CloseExecutionContext("agent", root.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.ErrorCode != CodeContextClosed {
			t.Fatalf("response = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("child pending call did not fail")
	}
	if _, err := h.ExecutionContextSnapshot(child.ID); !errors.Is(err, ErrContextNotFound) {
		t.Fatalf("child snapshot error = %v, want not found", err)
	}
}
