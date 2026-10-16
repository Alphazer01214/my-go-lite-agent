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
	started chan struct{}
	release chan struct{}
	done    chan struct{}
}

func (b *blockingCallBackend) CallWithEvents(
	from string,
	capability string,
	method string,
	payload json.RawMessage,
	onEvent func(*protocol.Frame),
) (*protocol.Frame, error) {
	close(b.started)
	<-b.release
	close(b.done)
	return &protocol.Frame{
		Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameResponse,
		From: "demo", Capability: capability, Method: method,
	}, nil
}

func TestBrowserCallDisconnectDoesNotCancelBackend(t *testing.T) {
	backend := &blockingCallBackend{
		started: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
	server := httptest.NewServer(NewServer(nil, backend).Handler())
	defer server.Close()

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
	case <-backend.done:
		t.Fatal("backend call ended before release")
	case <-time.After(50 * time.Millisecond):
	}

	close(backend.release)
	select {
	case <-backend.done:
	case <-time.After(time.Second):
		t.Fatal("backend call did not finish after release")
	}
}
