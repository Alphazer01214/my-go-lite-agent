package pluginsdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestCallRejectsNilContext(t *testing.T) {
	p, _, _ := newPipePlugin(t)
	_, err := p.Call(nil, "demo", "run", json.RawMessage(`{}`))
	if !errors.Is(err, ErrContextRequired) {
		t.Fatalf("error = %v, want context required", err)
	}
}

func TestCallReturnsContextCancellation(t *testing.T) {
	p, _, outR := newPipePlugin(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Call(ctx, "demo", "run", json.RawMessage(`{}`))
	if err == nil || Code(err) != CodeCallCancelled {
		t.Fatalf("error = %v, want call_cancelled", err)
	}
	if info, err := outR.Stat(); err != nil {
		t.Fatal(err)
	} else if info.Size() != 0 {
		t.Fatalf("canceled call wrote %d bytes", info.Size())
	}
}

func TestCallReturnsDeadlineExceeded(t *testing.T) {
	p, inW, outR := newPipePlugin(t)
	go p.Listen()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := p.Call(ctx, "demo", "run", json.RawMessage(`{}`))
		done <- err
	}()

	req, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if req.Type != protocol.FrameRequest {
		t.Fatalf("request = %#v", req)
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = protocol.WriteFrame(inW, &protocol.Frame{
			Version: protocol.FrameVersion, ID: req.ID, Type: protocol.FrameResponse,
			Capability: "demo", Method: "run", Payload: json.RawMessage(`{}`),
		})
	}()
	_ = inW
	select {
	case err := <-done:
		if err == nil || Code(err) != CodeCallDeadlineExceeded {
			t.Fatalf("error = %v, want call_deadline_exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline call did not return")
	}
}

func TestNestedCallUsesSameContext(t *testing.T) {
	p, inW, outR := newPipePlugin(t)
	p.Register("outer", "run", NewHandler(func(ctx context.Context, req *Request) (json.RawMessage, error) {
		return p.Call(ctx, "inner", "run", json.RawMessage(`{}`))
	}))
	go p.Listen()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := p.Call(ctx, "outer", "run", json.RawMessage(`{}`))
		done <- err
	}()

	outerReq, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if outerReq.Capability != "outer" {
		t.Fatalf("outer request = %#v", outerReq)
	}
	if err := protocol.WriteFrame(inW, outerReq); err != nil {
		t.Fatal(err)
	}
	innerReq, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if innerReq.Capability != "inner" {
		t.Fatalf("inner request = %#v", innerReq)
	}

	go func() {
		time.Sleep(45 * time.Millisecond)
		_ = protocol.WriteFrame(inW, &protocol.Frame{
			Version: protocol.FrameVersion, ID: innerReq.ID, Type: protocol.FrameResponse,
			Capability: "inner", Method: "run", Payload: json.RawMessage(`{}`),
		})
		time.Sleep(10 * time.Millisecond)
		_ = protocol.WriteFrame(inW, &protocol.Frame{
			Version: protocol.FrameVersion, ID: outerReq.ID, Type: protocol.FrameResponse,
			Capability: "outer", Method: "run", Payload: json.RawMessage(`{}`),
		})
	}()
	select {
	case err := <-done:
		if err == nil || Code(err) != CodeCallDeadlineExceeded {
			t.Fatalf("error = %v, want nested deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("nested call did not inherit deadline")
	}
}
