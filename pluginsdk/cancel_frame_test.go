package pluginsdk

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestContextCancelSendsCancelFrameAndKeepsFinalResponse(t *testing.T) {
	p, inW, outR := newPipePlugin(t)
	go p.Listen()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.Call(ctx, "demo", "run", json.RawMessage(`{}`))
		done <- err
	}()

	req, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	cancelFrame, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if cancelFrame.Type != protocol.FrameCancel || cancelFrame.ID != req.ID || cancelFrame.Reason != "cancelled" {
		t.Fatalf("cancel frame = %#v", cancelFrame)
	}

	if err := protocol.WriteFrame(inW, &protocol.Frame{
		Version: protocol.FrameVersion, ID: req.ID, Type: protocol.FrameResponse,
		Capability: "demo", Method: "run", Payload: json.RawMessage(`{"ok":true}`),
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || Code(err) != CodeCallCancelled {
			t.Fatalf("error = %v, want call_cancelled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not complete after final response")
	}
}

func TestCancelFrameCancelsHandlerContext(t *testing.T) {
	p, inW, outR := newPipePlugin(t)
	p.Register("demo", "run", NewHandler(func(ctx context.Context, _ *Request) (json.RawMessage, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	go p.Listen()

	if err := protocol.WriteFrame(inW, &protocol.Frame{
		Version: protocol.FrameVersion, ID: "in-1", Type: protocol.FrameRequest,
		Capability: "demo", Method: "run", Payload: json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := protocol.WriteFrame(inW, &protocol.Frame{
		Version: protocol.FrameVersion, ID: "in-1", Type: protocol.FrameCancel,
		Capability: "demo", Method: "run", Reason: "cancelled",
	}); err != nil {
		t.Fatal(err)
	}

	res, err := protocol.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if res.Type != protocol.FrameResponse || res.ID != "in-1" || res.ErrorCode != CodeCallCancelled {
		t.Fatalf("response = %#v", res)
	}
}

func TestRequestProceedRejectsSecondCall(t *testing.T) {
	req := &Request{
		proceed: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		},
	}
	if _, err := req.Proceed(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	_, err := req.Proceed(context.Background(), json.RawMessage(`{}`))
	if err == nil || Code(err) != CodeFlowAlreadyProceeded {
		t.Fatalf("error = %v, want flow_already_proceeded", err)
	}
}
