package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCancelAbortsUpstreamStream(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	p := newPlugin()
	// Route the default model at the stub server.
	p.mu.Lock()
	p.cfg.Models[0].BaseURL = server.URL
	p.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := p.complete(context.Background(), completeIn{
			CallID:   "call-1",
			Messages: []chatMessage{{Role: "user", Content: "hello"}},
		}, nil)
		done <- err
	}()

	<-started
	if !p.cancelCall("call-1") {
		t.Fatal("cancelCall = false")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("complete error = nil after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("complete did not return after cancellation")
	}
}

func TestCancelBeforeCompleteRegistration(t *testing.T) {
	p := newPlugin()
	// Nothing is in flight yet, so the cancel is buffered rather than reported
	// as a success against a call that does not exist.
	if p.cancelCall("early-call") {
		t.Fatal("cancelCall = true for a call that has not registered")
	}
	cancelled := false
	p.registerCall("early-call", func() { cancelled = true })
	if !cancelled {
		t.Fatal("pre-cancelled call was not cancelled on registration")
	}
}

func TestCancelBufferIsBounded(t *testing.T) {
	p := newPlugin()
	for i := 0; i < cancelledWindow*2; i++ {
		if p.cancelCall(string(rune('a'+i%26)) + string(rune('0'+i%10)) + "-" + itoa(i)) {
			t.Fatalf("cancelCall = true for unregistered call %d", i)
		}
	}
	p.mu.Lock()
	buffered := len(p.cancelled)
	p.mu.Unlock()
	if buffered > cancelledWindow {
		t.Fatalf("cancel buffer grew to %d, want <= %d", buffered, cancelledWindow)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
