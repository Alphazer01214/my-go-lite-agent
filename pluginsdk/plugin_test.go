package pluginsdk

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func newPipePlugin(t *testing.T) (*Plugin, *os.File, *os.File) {
	t.Helper()
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = inR.Close()
		_ = inW.Close()
		_ = outR.Close()
		_ = outW.Close()
	})
	p := NewPlugin("tester")
	p.stdin = inR
	p.stdout = outW
	return p, inW, outR
}

func TestDispatchUnknownMethod(t *testing.T) {
	p, _, outR := newPipePlugin(t)

	go p.Dispatch(&protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         "tester-1",
		Type:       protocol.FrameRequest,
		Capability: "nope",
		Method:     "missing",
	})

	res, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != protocol.FrameResponse || res.ID != "tester-1" {
		t.Fatalf("bad res envelope: %+v", res)
	}
	if res.ErrorCode != CodeMethodNotFound {
		t.Fatalf("want method_not_found, got %q", res.ErrorCode)
	}
}

func TestDispatchHandlerErrorAndOK(t *testing.T) {
	p, _, outR := newPipePlugin(t)
	p.Register("demo", "fail", func(req *Request) (json.RawMessage, error) {
		return nil, ErrCode("bad_arguments", "nope")
	})
	p.Register("demo", "ok", func(req *Request) (json.RawMessage, error) {
		return json.RawMessage(`{"v":1}`), nil
	})

	go p.Dispatch(&protocol.Frame{
		ID: "a", Type: protocol.FrameRequest, Capability: "demo", Method: "fail",
	})
	res, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "bad_arguments" || res.ErrorMsg == "" {
		t.Fatalf("want bad_arguments, got %+v", res)
	}

	go p.Dispatch(&protocol.Frame{
		ID: "b", Type: protocol.FrameRequest, Capability: "demo", Method: "ok",
		Payload: json.RawMessage(`{}`),
	})
	res, err = protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if res.ErrorCode != "" || res.ErrorMsg != "" || string(res.Payload) != `{"v":1}` {
		t.Fatalf("want success payload, got %+v", res)
	}
}

func TestListenDispatchAndComplete(t *testing.T) {
	p, inW, outR := newPipePlugin(t)
	p.Register("demo", "echo", func(req *Request) (json.RawMessage, error) {
		return req.Payload, nil
	})

	done := make(chan error, 1)
	go func() { done <- p.Listen() }()

	if err := protocol.WriteFrame(inW, &protocol.Frame{
		ID: "in-1", Type: protocol.FrameRequest, Capability: "demo", Method: "echo",
		Payload: json.RawMessage(`{"x":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != "in-1" || string(res.Payload) != `{"x":1}` {
		t.Fatalf("dispatch via Listen failed: %+v", res)
	}

	callDone := make(chan error, 1)
	go func() {
		_, err := p.Call("other", "go", json.RawMessage(`{}`))
		callDone <- err
	}()

	req, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if req.Type != protocol.FrameRequest || req.Capability != "other" {
		t.Fatalf("bad outbound req: %+v", req)
	}
	if err := protocol.WriteFrame(inW, &protocol.Frame{
		ID: req.ID, Type: protocol.FrameResponse, Capability: "other", Method: "go",
		Payload: json.RawMessage(`{"ok":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-callDone:
		if err != nil {
			t.Fatalf("call should succeed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call did not complete")
	}

	_ = inW.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Listen should return read error after stdin close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Listen did not return")
	}
}

func TestFailUnblocksCall(t *testing.T) {
	p, _, _ := newPipePlugin(t)

	callDone := make(chan error, 1)
	go func() {
		_, err := p.Call("other", "go", json.RawMessage(`{}`))
		callDone <- err
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		n := len(p.pending)
		p.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.fail()

	select {
	case err := <-callDone:
		if err == nil || err.Error() != "call other.go: connection closed" {
			t.Fatalf("want connection closed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Call still blocked after fail")
	}
}

func TestCodeHelpers(t *testing.T) {
	if Code(nil) != "" || Code(errors.New("x")) != "" {
		t.Fatal("plain errors have no code")
	}
	err := ErrCode("bad_payload", "broken")
	if Code(err) != "bad_payload" || err.Error() != "broken" {
		t.Fatalf("ErrCode/Code mismatch: %v / %s", err, Code(err))
	}
	wrapped := errors.New("wrap")
	_ = wrapped
	w := error(ErrCode("bad_arguments", "limit"))
	w = &wrapErr{w}
	if Code(w) != "bad_arguments" {
		t.Fatalf("Code should unwrap, got %q", Code(w))
	}
}

type wrapErr struct{ inner error }

func (e *wrapErr) Error() string { return e.inner.Error() }
func (e *wrapErr) Unwrap() error { return e.inner }
