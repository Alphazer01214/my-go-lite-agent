package pluginsdk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
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

func newDispatchTestPlugin(t *testing.T, handler HandleFunc) (*Plugin, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout.frames")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create frame sink: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	p := &Plugin{
		name:             "test",
		handlers:         map[string]Handler{"example.run": {handleFunc: handler}},
		pending:          make(map[string]*outboundCall),
		inbound:          make(map[string]*inboundCall),
		inboundRecentSet: make(map[string]bool),
		stdout:           file,
	}
	return p, path
}

func dispatchFrame(id string) *protocol.Frame {
	return &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         id,
		Type:       protocol.FrameRequest,
		Capability: "example",
		Method:     "run",
		Payload:    json.RawMessage(`{}`),
	}
}

func readWrittenFrames(t *testing.T, path string) []*protocol.Frame {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open frame sink: %v", err)
	}
	defer file.Close()
	var frames []*protocol.Frame
	for {
		frame, err := protocol.ReadFrame(file)
		if err != nil {
			return frames
		}
		frames = append(frames, frame)
	}
}

func TestProceedTerminalSkipsResponse(t *testing.T) {
	p, path := newDispatchTestPlugin(t, func(ctx context.Context, req *Request) (json.RawMessage, error) {
		req.proceed = func(context.Context, json.RawMessage) error { return nil }
		return json.RawMessage(`{"ignored":true}`), req.Proceed(ctx, json.RawMessage(`{"step":2}`))
	})
	p.dispatch(dispatchFrame("plugin-1"))

	if frames := readWrittenFrames(t, path); len(frames) != 0 {
		t.Fatalf("dispatch wrote %d frames after a terminal Proceed, want 0", len(frames))
	}
}

func TestProceedFailureFallsBackToResponse(t *testing.T) {
	const routeFailed = "route_failed"
	proceedErr := ErrCode(routeFailed, "no next hop")
	p, path := newDispatchTestPlugin(t, func(ctx context.Context, req *Request) (json.RawMessage, error) {
		req.proceed = func(context.Context, json.RawMessage) error { return proceedErr }
		if err := req.Proceed(ctx, nil); err != nil {
			return nil, err
		}
		return nil, nil
	})
	p.dispatch(dispatchFrame("plugin-2"))

	frames := readWrittenFrames(t, path)
	if len(frames) != 1 {
		t.Fatalf("dispatch wrote %d frames after a failed Proceed, want 1", len(frames))
	}
	if frames[0].ErrorCode != routeFailed || frames[0].ID != "plugin-2" {
		t.Fatalf("response = %s/%s, want %s/plugin-2", frames[0].ErrorCode, frames[0].ID, routeFailed)
	}
}

func TestProceedRunsAtMostOnce(t *testing.T) {
	var calls int
	req := &Request{proceed: func(context.Context, json.RawMessage) error {
		calls++
		return nil
	}}
	if err := req.Proceed(context.Background(), nil); err != nil {
		t.Fatalf("first Proceed: %v", err)
	}
	if !req.proceeded.Load() {
		t.Fatal("proceeded is false after a successful Proceed")
	}
	err := req.Proceed(context.Background(), nil)
	if Code(err) != CodeFlowAlreadyProceeded {
		t.Fatalf("second Proceed error code = %q, want %q", Code(err), CodeFlowAlreadyProceeded)
	}
	if calls != 1 {
		t.Fatalf("proceed backend called %d times, want 1", calls)
	}
}
