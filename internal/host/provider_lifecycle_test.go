package host

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func TestProviderSnapshotGroupsInstancesByPluginName(t *testing.T) {
	h := newTestHost()
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{{
		Manifest: plugin.Manifest{Name: "demo", Version: "1.0.0", Provides: []string{"agent"}},
	}}}
	h.plugins["demo-b"] = &proc{
		instanceID: "demo-b", pluginName: "demo", gen: 2,
		state: ProviderStateLoading,
	}
	h.plugins["demo-a"] = &proc{
		instanceID: "demo-a", pluginName: "demo", gen: 1,
		state: ProviderStateActive,
		registrations: []RegistrationSnapshot{
			{Capability: "agent", Method: "run"},
		},
	}

	got := h.PluginSnapshot()
	if len(got) != 1 {
		t.Fatalf("snapshot = %#v", got)
	}
	if got[0].Name != "demo" || got[0].State != ProviderStateActive || got[0].SelectedInstanceID != "demo-a" {
		t.Fatalf("plugin snapshot = %#v", got[0])
	}
	if len(got[0].Instances) != 2 {
		t.Fatalf("instances = %#v", got[0].Instances)
	}
	if got[0].Instances[0].InstanceID != "demo-a" || got[0].Instances[1].InstanceID != "demo-b" {
		t.Fatalf("instances = %#v", got[0].Instances)
	}
	if got[0].Instances[0].State != ProviderStateActive || got[0].Instances[1].State != ProviderStateLoading {
		t.Fatalf("instance states = %#v", got[0].Instances)
	}
}

func TestHostRegisterActivatesProviderAndRecordsRegistrations(t *testing.T) {
	h := newTestHost("demo")
	p := h.plugins["demo"]
	p.discovery.Manifest.Provides = []string{"agent"}
	p.state = ProviderStateLoading

	err := h.handleRequest("demo", &Frame{
		ID: "reg-1", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
		Payload: mustJSON(map[string]any{
			"handlers": []map[string]string{{"capability": "agent", "method": "run"}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.state != ProviderStateActive {
		t.Fatalf("state = %q, want %q", p.state, ProviderStateActive)
	}
	if len(p.registrations) != 1 || p.registrations[0].Capability != "agent" || p.registrations[0].Method != "run" {
		t.Fatalf("registrations = %#v", p.registrations)
	}
}

func TestProviderCallGateRejectsNonActiveStates(t *testing.T) {
	for _, state := range []string{ProviderStateLoading, ProviderStateStopping, ProviderStateFailed} {
		t.Run(state, func(t *testing.T) {
			h := newTestHost("caller", "target")
			target := h.plugins["target"]
			target.state = state
			h.routes["agent.run"] = "target"

			err := h.handleRequest("caller", &Frame{
				ID: "call-1", Type: FrameRequest, Capability: "agent", Method: "run",
				Payload: mustJSON(map[string]any{}),
			})
			if err == nil {
				t.Fatal("non-active provider accepted a new call")
			}
			frames := drainFrames(t, h, "caller")
			if len(frames) != 1 || frames[0].ErrorCode != CodePluginNotReady {
				t.Fatalf("response = %#v", frames)
			}
			if frames := drainFrames(t, h, "target"); len(frames) != 0 {
				t.Fatalf("target received frames: %#v", frames)
			}
		})
	}
}

func TestStopPluginDrainsExistingCalls(t *testing.T) {
	h := newTestHost("demo")
	h.plugins["demo"].pluginName = "demo"
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{{
		Manifest: plugin.Manifest{Name: "demo", Entry: "demo"},
	}}}
	response := make(chan *Frame, 1)
	h.pending["host-1"] = &wait{
		kind: WaitHost, target: "demo", targetPlugin: "demo",
		frameID: "host-1", capability: "agent", method: "run", resCh: response,
	}

	done := make(chan error, 1)
	go func() { done <- h.StopPlugin("demo") }()
	go func() {
		time.Sleep(10 * time.Millisecond)
		h.mu.Lock()
		delete(h.pending, "host-1")
		h.mu.Unlock()
		response <- &Frame{ID: "host-1", Type: FrameResponse}
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("StopPlugin did not finish")
	}
	select {
	case frame := <-response:
		if frame.ErrorCode != "" {
			t.Fatalf("existing call failed during stopping: %#v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("existing call did not complete")
	}
}

func TestProviderCrashRecordsFailureAndClosesPending(t *testing.T) {
	h := newTestHost("demo")
	p := h.plugins["demo"]
	p.state = ProviderStateActive
	response := make(chan *Frame, 1)
	h.pending["host-1"] = &wait{
		kind: WaitHost, target: "demo", targetPlugin: "demo",
		frameID: "host-1", capability: "agent", method: "run", resCh: response,
	}

	h.markUnhealthy("demo", p.gen)

	if p.state != ProviderStateFailed || p.failureCode != CodePluginDown || p.failureMessage == "" {
		t.Fatalf("provider = %#v", p)
	}
	select {
	case frame := <-response:
		if frame.ErrorCode != CodePluginDown {
			t.Fatalf("response = %#v", frame)
		}
	default:
		t.Fatal("pending call was not closed")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRestartPluginInstanceIncrementsGeneration(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	discovery := plugin.Discovery{
		Dir: filepath.Dir(executable),
		Manifest: plugin.Manifest{
			Name: "helper", Entry: filepath.Base(executable),
		},
	}
	h := newTestHost()
	if err := h.launchInstance("helper", discovery, "cfg-1"); err != nil {
		t.Fatal(err)
	}

	restarted, err := h.RestartPluginInstance("helper")
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Generation != 2 || restarted.ConfigRevision != "cfg-1" || restarted.State != ProviderStateLoading {
		t.Fatalf("restarted = %#v", restarted)
	}
	if err := h.ClosePluginInstance("helper"); err != nil {
		t.Fatal(err)
	}
}
