package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

type blockingCallBackend struct {
	started   chan struct{}
	release   chan struct{}
	completed chan struct{}
	canceled  chan struct{}
}

func (b *blockingCallBackend) CallWithEvents(
	ctx context.Context,
	from string,
	capability string,
	method string,
	payload json.RawMessage,
	onEvent func(*protocol.Frame),
) (*protocol.Frame, error) {
	close(b.started)
	select {
	case <-ctx.Done():
		close(b.canceled)
		return nil, ctx.Err()
	case <-b.release:
		close(b.completed)
	}
	return &protocol.Frame{
		Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameResponse,
		From: "demo", Capability: capability, Method: method,
	}, nil
}

func TestBrowserCallDisconnectLeavesBackendRunning(t *testing.T) {
	backend := &blockingCallBackend{
		started:   make(chan struct{}),
		release:   make(chan struct{}),
		completed: make(chan struct{}),
		canceled:  make(chan struct{}),
	}
	server := httptest.NewServer(NewServer(nil, backend).Handler())
	defer server.Close()

	release := func() {
		select {
		case <-backend.release:
		default:
			close(backend.release)
		}
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL+"/bridge/call",
		strings.NewReader(`{"version":1,"id":"ui-call-1","type":"req","capability":"demo","method":"run","payload":{}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	<-backend.started
	cancel()
	_ = response.Body.Close()

	select {
	case <-backend.canceled:
		t.Fatal("backend call was canceled by browser disconnect")
	case <-time.After(200 * time.Millisecond):
	}

	release()
	select {
	case <-backend.completed:
	case <-time.After(time.Second):
		t.Fatal("backend call did not complete after release")
	}
}
