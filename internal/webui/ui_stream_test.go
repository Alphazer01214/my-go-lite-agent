package webui

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

func TestUIStreamDeliversStateAndDiagnostic(t *testing.T) {
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	server := NewServer(registry, nil)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/bridge/ui", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	lines := make(chan string, 2)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			lines <- line
		}
	}()

	if _, err := server.PublishState("demo", json.RawMessage(`{"mount_id":"chat","state":{"revision":1}}`)); err != nil {
		t.Fatal(err)
	}
	server.PushDiagnostic(plugin.Diagnostic{Plugin: "demo", Code: "warning", Message: "hello"})

	for _, want := range []string{"state", "warning"} {
		select {
		case line := <-lines:
			var envelope uiEnvelope
			if err := json.Unmarshal([]byte(line), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Type == "ui" && envelope.Kind != want {
				t.Fatalf("envelope = %#v, want kind %s", envelope, want)
			}
			if envelope.Type == "diagnostic" && (envelope.Diagnostic == nil || envelope.Diagnostic.Code != want) {
				t.Fatalf("envelope = %#v, want diagnostic %s", envelope, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

func TestUIPublishIsNonBlockingWhenOffline(t *testing.T) {
	server := NewServer(nil, nil)
	done := make(chan error, 1)
	go func() {
		_, err := server.PublishState("demo", json.RawMessage(`{"mount_id":"chat","state":{}}`))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("offline publish should reject missing mount")
		}
	case <-time.After(time.Second):
		t.Fatal("offline publish blocked")
	}
}
