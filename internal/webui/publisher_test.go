package webui

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

func testPublisher(t *testing.T) (*Server, *uiConnection) {
	t.Helper()
	discovery := testDiscovery(t, plugin.UIComponent{Tag: "demo-chat", MountID: "chat", Slot: "center"})
	registry, _ := NewRegistry(plugin.Discoveries{Plugins: []plugin.Discovery{discovery}})
	server := NewServer(registry, nil)
	connection := newUIConnection()
	server.replaceUIConnection(connection)
	return server, connection
}

func readUIEnvelope(t *testing.T, connection *uiConnection) *uiEnvelope {
	t.Helper()
	select {
	case envelope := <-connection.queue:
		return envelope
	case <-time.After(time.Second):
		t.Fatal("UI envelope was not queued")
		return nil
	}
}

func TestServerPublishesStateCommandAndDiagnostic(t *testing.T) {
	server, connection := testPublisher(t)

	if _, err := server.PublishState("demo", json.RawMessage(`{"mount_id":"chat","state":{"revision":2}}`)); err != nil {
		t.Fatal(err)
	}
	state := readUIEnvelope(t, connection)
	if state.Type != "ui" || state.Kind != "state" || state.Mount.Plugin != "demo" || state.Mount.ID != "chat" {
		t.Fatalf("state envelope = %#v", state)
	}
	if string(state.Payload) != `{"revision":2}` {
		t.Fatalf("state payload = %s", state.Payload)
	}

	if _, err := server.PublishCommand("demo", json.RawMessage(`{"mount_id":"chat","command":"scroll","payload":{"position":"bottom"}}`)); err != nil {
		t.Fatal(err)
	}
	command := readUIEnvelope(t, connection)
	if command.Kind != "command" || command.Command != "scroll" || command.Mount.ID != "chat" {
		t.Fatalf("command envelope = %#v", command)
	}

	server.PushDiagnostic(plugin.Diagnostic{Plugin: "demo", Code: "warning", Message: "hello"})
	diagnostic := readUIEnvelope(t, connection)
	if diagnostic.Type != "diagnostic" || diagnostic.Diagnostic == nil || diagnostic.Diagnostic.Code != "warning" {
		t.Fatalf("diagnostic envelope = %#v", diagnostic)
	}

	bootstrap := server.Bootstrap()
	if bootstrap.Diagnostics[0].Code != "warning" {
		t.Fatalf("diagnostic snapshot = %#v", bootstrap.Diagnostics)
	}
}

func TestServerValidatesUIPublishOwnership(t *testing.T) {
	server, _ := testPublisher(t)

	_, err := server.PublishState("other", json.RawMessage(`{"mount_id":"chat","state":{}}`))
	if !errors.Is(err, protocol.ErrUIOwnerMismatch) {
		t.Fatalf("owner error = %v", err)
	}
	_, err = server.PublishState("demo", json.RawMessage(`{"mount_id":"missing","state":{}}`))
	if !errors.Is(err, protocol.ErrUINotMounted) {
		t.Fatalf("mount error = %v", err)
	}
	_, err = server.PublishState("demo", json.RawMessage(`{"mount_id":"chat"}`))
	if !errors.Is(err, host.ErrBadPayload) {
		t.Fatalf("payload error = %v", err)
	}
	_, err = server.PublishCommand("demo", json.RawMessage(`{"mount_id":"chat","command":""}`))
	if !errors.Is(err, host.ErrBadPayload) {
		t.Fatalf("command error = %v", err)
	}
}

func TestServerUIConnectionReplacementStopsPrevious(t *testing.T) {
	server, first := testPublisher(t)
	second := newUIConnection()
	server.replaceUIConnection(second)

	select {
	case <-first.done:
	case <-time.After(time.Second):
		t.Fatal("previous UI connection was not stopped")
	}
}
