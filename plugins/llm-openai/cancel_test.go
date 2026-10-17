package main

import (
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
	p.cfg.BaseURL = server.URL
	done := make(chan error, 1)
	go func() {
		_, err := p.complete(completeIn{
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
	if !p.cancelCall("early-call") {
		t.Fatal("cancelCall = false")
	}
	cancelled := false
	p.registerCall("early-call", func() { cancelled = true })
	if !cancelled {
		t.Fatal("pre-cancelled call was not cancelled on registration")
	}
}
