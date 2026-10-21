package pluginsdk

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestCallDeadlineDoesNotDependOnCancelWrite(t *testing.T) {
	p, _, _ := newPipePlugin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Call(ctx, "demo", "run", json.RawMessage(`{}`))
	if code := Code(err); code != CodeCallDeadlineExceeded {
		t.Fatalf("Call error code = %q, want %q (err=%v)", code, CodeCallDeadlineExceeded, err)
	}

	p.mu.Lock()
	pending := len(p.pending)
	p.mu.Unlock()
	if pending != 0 {
		t.Fatalf("pending calls = %d after deadline, want 0", pending)
	}
}

func TestBlockedStdoutDoesNotBlockComplete(t *testing.T) {
	p, _, outR := newPipePlugin(t)

	payload, err := json.Marshal(strings.Repeat("x", 4<<20))
	if err != nil {
		t.Fatal(err)
	}
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- p.writeFrame(&protocol.Frame{
			Version: protocol.FrameVersion,
			ID:      "blocked-write",
			Type:    protocol.FrameRequest,
			Payload: payload,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if !p.writeMu.TryLock() {
			break
		}
		p.writeMu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("stdout write did not block")
		}
		time.Sleep(10 * time.Millisecond)
	}

	call := &outboundCall{id: "pending-1", ch: make(chan *protocol.Frame, 1)}
	p.mu.Lock()
	p.pending[call.id] = call
	p.mu.Unlock()

	completeDone := make(chan struct{})
	go func() {
		p.complete(&protocol.Frame{
			Version: protocol.FrameVersion,
			ID:      call.id,
			Type:    protocol.FrameResponse,
		})
		close(completeDone)
	}()
	select {
	case <-completeDone:
	case <-time.After(time.Second):
		t.Fatal("complete blocked behind the stdout write")
	}

	_ = outR.Close()
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("blocked stdout write did not exit after the reader closed")
	}
}
