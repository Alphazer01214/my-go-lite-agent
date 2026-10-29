package host

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

const pipelineTestTimeout = 2 * time.Second

type fakeInstance struct {
	in     *io.PipeReader
	out    *io.PipeWriter
	frames chan *Frame
}

type testHost struct {
	*Host
	instances map[string]*fakeInstance
}

// newTestHost builds a Host over scripted in-process "plugins": the test reads
// the frames Host writes on each instance's pipe and answers with routeFrame.
func newTestHost(t *testing.T, manifests []plugin.Manifest) *testHost {
	t.Helper()
	th := &testHost{
		Host:      New(plugin.Discoveries{}, func(string) {}),
		instances: make(map[string]*fakeInstance),
	}
	for _, manifest := range manifests {
		th.Host.discoveries.Plugins = append(th.Host.discoveries.Plugins, plugin.Discovery{Manifest: manifest})
	}
	for _, manifest := range manifests {
		reader, writer := io.Pipe()
		instance := &fakeInstance{in: reader, out: writer, frames: make(chan *Frame, 16)}
		th.instances[manifest.Name] = instance
		th.Host.plugins[manifest.Name] = &proc{
			discovery:  plugin.Discovery{Manifest: manifest},
			instanceID: manifest.Name,
			pluginName: manifest.Name,
			stdin:      writer,
			gen:        1,
			done:       make(chan struct{}),
			state:      ProviderStateActive,
		}
		th.Host.gen[manifest.Name] = 1
		go func() {
			for {
				frame, err := ReadFrame(reader)
				if err != nil {
					return
				}
				instance.frames <- frame
			}
		}()
	}
	t.Cleanup(func() {
		for _, instance := range th.instances {
			_ = instance.out.Close()
		}
	})
	return th
}

func consumerManifest(name string, priority int) plugin.Manifest {
	return plugin.Manifest{
		Name:     name,
		Priority: priority,
		Consumes: []plugin.ConsumesMethod{{Capability: "x", Method: "y"}},
	}
}

func providerManifest(name string, capabilities ...string) plugin.Manifest {
	return plugin.Manifest{Name: name, Provides: capabilities}
}

func (th *testHost) setRoute(capability, method, owner string) {
	th.Host.mu.Lock()
	th.Host.routes[capability+"."+method] = owner
	th.Host.mu.Unlock()
}

func (th *testHost) send(from string, frame *Frame) {
	th.Host.routeFrame(from, 1, frame)
}

func (th *testHost) next(t *testing.T, name string) *Frame {
	t.Helper()
	select {
	case frame := <-th.instances[name].frames:
		return frame
	case <-time.After(pipelineTestTimeout):
		t.Fatalf("%s received no frame", name)
		return nil
	}
}

func (th *testHost) noFrame(t *testing.T, name string) {
	t.Helper()
	select {
	case frame := <-th.instances[name].frames:
		t.Fatalf("%s received unexpected frame id=%s type=%s", name, frame.ID, frame.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

func businessRequest(id string) *Frame {
	return &Frame{
		Version:    FrameVersion,
		ID:         id,
		Type:       FrameRequest,
		Capability: "x",
		Method:     "y",
		Payload:    json.RawMessage(`{"n":1}`),
	}
}

func hostProceedRequest(id, callID string, payload json.RawMessage) *Frame {
	body := map[string]any{"call_id": callID}
	if len(payload) != 0 {
		body["payload"] = payload
	}
	encoded, _ := json.Marshal(body)
	return &Frame{
		Version:    FrameVersion,
		ID:         id,
		Type:       FrameRequest,
		Capability: HostCapability,
		Method:     HostMethodProceed,
		Payload:    encoded,
	}
}

func assertProceedAck(t *testing.T, frame *Frame, id string) {
	t.Helper()
	if frame.Type != FrameResponse || frame.ID != id {
		t.Fatalf("ack = type %s id %s, want res %s", frame.Type, frame.ID, id)
	}
	if frame.ErrorCode != "" || string(frame.Payload) != `{"accepted":true}` {
		t.Fatalf("ack payload = %s error = %s, want accepted", frame.Payload, frame.ErrorCode)
	}
}

func TestProceedTwoConsumersReachOriginalCaller(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{
		providerManifest("caller"),
		consumerManifest("audit", 20),
		consumerManifest("guard", 10),
		providerManifest("provider", "x"),
	})
	th.setRoute("x", "y", "provider")

	th.send("caller", businessRequest("caller-1"))
	hop1 := th.next(t, "audit")
	if hop1.Type != FrameRequest || string(hop1.Payload) != `{"n":1}` {
		t.Fatalf("first hop = %s %s, want req with original payload", hop1.Type, hop1.Payload)
	}

	th.send("audit", hostProceedRequest("audit-call", hop1.ID, json.RawMessage(`{"n":2}`)))
	assertProceedAck(t, th.next(t, "audit"), "audit-call")

	hop2 := th.next(t, "guard")
	if string(hop2.Payload) != `{"n":2}` {
		t.Fatalf("second hop payload = %s, want {\"n\":2}", hop2.Payload)
	}

	th.send("guard", hostProceedRequest("guard-call", hop2.ID, nil))
	assertProceedAck(t, th.next(t, "guard"), "guard-call")

	hop3 := th.next(t, "provider")
	if string(hop3.Payload) != `{"n":2}` {
		t.Fatalf("terminal hop payload = %s, want the last rewritten payload", hop3.Payload)
	}

	th.send("provider", &Frame{
		Version:    FrameVersion,
		ID:         hop3.ID,
		Type:       FrameEvent,
		Capability: "x",
		Method:     "y",
		Payload:    json.RawMessage(`{"delta":1}`),
	})
	event := th.next(t, "caller")
	if event.Type != FrameEvent || event.ID != "caller-1" {
		t.Fatalf("event = %s id %s, want evt with original id", event.Type, event.ID)
	}

	th.send("provider", &Frame{
		Version: FrameVersion,
		ID:      hop3.ID,
		Type:    FrameResponse,
		Payload: json.RawMessage(`{"ok":true}`),
	})
	result := th.next(t, "caller")
	if result.Type != FrameResponse || result.ID != "caller-1" || result.From != "provider" {
		t.Fatalf("result = type %s id %s from %s, want res caller-1 from provider",
			result.Type, result.ID, result.From)
	}
	if string(result.Payload) != `{"ok":true}` {
		t.Fatalf("result payload = %s, want {\"ok\":true}", result.Payload)
	}

	// Consumers only ever see the internal ack, never the business response.
	th.noFrame(t, "audit")
	th.noFrame(t, "guard")

	// A Consumer that writes a late business res for the forwarded request is
	// ignored: the caller has already received the terminal response.
	th.send("audit", &Frame{
		Version: FrameVersion,
		ID:      hop1.ID,
		Type:    FrameResponse,
		Payload: json.RawMessage(`{"late":true}`),
	})
	th.noFrame(t, "caller")
}

func TestProceedConsumerOwnCallStaysWithConsumer(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{
		providerManifest("caller"),
		consumerManifest("audit", 10),
		providerManifest("provider", "x", "helper"),
	})
	th.setRoute("x", "y", "provider")
	th.setRoute("helper", "work", "provider")

	th.send("caller", businessRequest("caller-1"))
	hop1 := th.next(t, "audit")

	th.send("audit", &Frame{
		Version:    FrameVersion,
		ID:         "audit-call-1",
		Type:       FrameRequest,
		Capability: "helper",
		Method:     "work",
		Payload:    json.RawMessage(`{}`),
	})
	helperHop := th.next(t, "provider")
	th.send("provider", &Frame{
		Version: FrameVersion,
		ID:      helperHop.ID,
		Type:    FrameResponse,
		Payload: json.RawMessage(`{"ok":true}`),
	})
	own := th.next(t, "audit")
	if own.ID != "audit-call-1" || own.ErrorCode != "" {
		t.Fatalf("consumer own call = id %s code %s, want audit-call-1 success", own.ID, own.ErrorCode)
	}

	th.send("audit", hostProceedRequest("audit-call", hop1.ID, nil))
	assertProceedAck(t, th.next(t, "audit"), "audit-call")
	hop2 := th.next(t, "provider")
	th.send("provider", &Frame{
		Version: FrameVersion,
		ID:      hop2.ID,
		Type:    FrameResponse,
		Payload: json.RawMessage(`{"ok":true}`),
	})
	result := th.next(t, "caller")
	if result.ID != "caller-1" || result.ErrorCode != "" {
		t.Fatalf("result = id %s code %s, want caller-1 success", result.ID, result.ErrorCode)
	}
}

func TestProceedProviderErrorReachesOriginalCaller(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{
		providerManifest("caller"),
		consumerManifest("audit", 10),
		providerManifest("provider", "x"),
	})
	th.setRoute("x", "y", "provider")

	th.send("caller", businessRequest("caller-1"))
	hop1 := th.next(t, "audit")
	th.send("audit", hostProceedRequest("audit-call", hop1.ID, nil))
	assertProceedAck(t, th.next(t, "audit"), "audit-call")

	hop2 := th.next(t, "provider")
	th.send("provider", &Frame{
		Version:   FrameVersion,
		ID:        hop2.ID,
		Type:      FrameResponse,
		ErrorCode: "provider_boom",
		ErrorMsg:  "provider failed",
	})

	result := th.next(t, "caller")
	if result.ID != "caller-1" || result.ErrorCode != "provider_boom" {
		t.Fatalf("result = id %s code %s, want caller-1/provider_boom", result.ID, result.ErrorCode)
	}
}

func TestProceedCancelFollowsCurrentHop(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{
		providerManifest("caller"),
		consumerManifest("audit", 10),
		providerManifest("provider", "x"),
	})
	th.setRoute("x", "y", "provider")

	th.send("caller", businessRequest("caller-1"))
	hop1 := th.next(t, "audit")
	th.send("audit", hostProceedRequest("audit-call", hop1.ID, nil))
	assertProceedAck(t, th.next(t, "audit"), "audit-call")
	hop2 := th.next(t, "provider")

	th.send("caller", &Frame{
		Version:    FrameVersion,
		ID:         "caller-1",
		Type:       FrameCancel,
		Capability: "x",
		Method:     "y",
		Reason:     "cancelled",
	})
	cancel := th.next(t, "provider")
	if cancel.Type != FrameCancel || cancel.ID != hop2.ID || cancel.Reason != "cancelled" {
		t.Fatalf("cancel = %s id %s reason %s, want cancel %s", cancel.Type, cancel.ID, cancel.Reason, hop2.ID)
	}

	th.send("provider", &Frame{
		Version:   FrameVersion,
		ID:        hop2.ID,
		Type:      FrameResponse,
		ErrorCode: CodeCallCancelled,
		ErrorMsg:  "call cancelled",
	})
	result := th.next(t, "caller")
	if result.ID != "caller-1" || result.ErrorCode != CodeCallCancelled {
		t.Fatalf("result = id %s code %s, want caller-1/%s", result.ID, result.ErrorCode, CodeCallCancelled)
	}
}

func TestProceedWithoutConsumerFallsBackToRouteOwner(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{
		providerManifest("caller"),
		providerManifest("provider", "x"),
	})
	th.setRoute("x", "y", "provider")

	th.send("caller", businessRequest("caller-1"))
	hop := th.next(t, "provider")
	if hop.Type != FrameRequest || string(hop.Payload) != `{"n":1}` {
		t.Fatalf("fallback hop = %s %s, want original req", hop.Type, hop.Payload)
	}
	th.send("provider", &Frame{
		Version: FrameVersion,
		ID:      hop.ID,
		Type:    FrameResponse,
		Payload: json.RawMessage(`{"ok":true}`),
	})
	result := th.next(t, "caller")
	if result.ID != "caller-1" || result.ErrorCode != "" {
		t.Fatalf("result = id %s code %s, want caller-1 success", result.ID, result.ErrorCode)
	}
}

func TestProceedWithoutRouteOwnerFails(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{providerManifest("caller")})

	th.send("caller", &Frame{
		Version:    FrameVersion,
		ID:         "caller-1",
		Type:       FrameRequest,
		Capability: "missing",
		Method:     "method",
		Payload:    json.RawMessage(`{}`),
	})
	result := th.next(t, "caller")
	if result.ID != "caller-1" || result.ErrorCode != CodeMethodNotFound {
		t.Fatalf("result = id %s code %s, want caller-1/%s", result.ID, result.ErrorCode, CodeMethodNotFound)
	}
}

func TestProceedMissingPipelineCallFails(t *testing.T) {
	th := newTestHost(t, []plugin.Manifest{providerManifest("audit")})

	th.send("audit", hostProceedRequest("audit-call", "fwd-404", nil))
	result := th.next(t, "audit")
	if result.ID != "audit-call" || result.ErrorCode != CodeMethodNotFound {
		t.Fatalf("result = id %s code %s, want audit-call/%s", result.ID, result.ErrorCode, CodeMethodNotFound)
	}
}
