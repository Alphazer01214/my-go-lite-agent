package pluginsdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

type WebUI struct {
	p *Plugin
}

// WebUI returns the plugin's UI publisher. Publish targets are always owned
// by the current plugin; mountID supplies only the instance suffix.
func (p *Plugin) WebUI() *WebUI {
	return &WebUI{p: p}
}

// uiPublishTimeout bounds one ui.state / ui.command round trip to Host. The
// publish is fire-and-forget from the plugin's side, so without a bound a Host
// that never replies would hang the publishing goroutine forever.
const uiPublishTimeout = 10 * time.Second

// SetState publishes a rebuildable UI state to the current plugin's mount.
// It uses a fresh bounded context: a UI publish outlives the handler that
// triggered it, so it must not inherit that handler's cancellation.
func (u *WebUI) SetState(mountID string, state json.RawMessage) error {
	if mountID == "" {
		return ErrCode(protocol.CodeBadArguments, "mount_id is required")
	}
	if len(bytes.TrimSpace(state)) == 0 || bytes.Equal(bytes.TrimSpace(state), []byte("null")) {
		return ErrCode(protocol.CodeBadArguments, "state is required")
	}

	payload, err := json.Marshal(struct {
		MountID string          `json:"mount_id"`
		State   json.RawMessage `json:"state"`
	}{MountID: mountID, State: state})
	if err != nil {
		return fmt.Errorf("encode ui state: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), uiPublishTimeout)
	defer cancel()
	_, err = u.p.Call(ctx, protocol.HostCapability, "ui.state", payload)
	return err
}

// SendCommand publishes a transient UI command to the current plugin's mount.
// Like SetState it publishes on its own bounded context.
func (u *WebUI) SendCommand(mountID string, command string, payload json.RawMessage) error {
	if mountID == "" {
		return ErrCode(protocol.CodeBadArguments, "mount_id is required")
	}
	if command == "" {
		return ErrCode(protocol.CodeBadArguments, "command is required")
	}
	if len(bytes.TrimSpace(payload)) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		payload = json.RawMessage(`{}`)
	}

	body, err := json.Marshal(struct {
		MountID string          `json:"mount_id"`
		Command string          `json:"command"`
		Payload json.RawMessage `json:"payload"`
	}{MountID: mountID, Command: command, Payload: payload})
	if err != nil {
		return fmt.Errorf("encode ui command: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), uiPublishTimeout)
	defer cancel()
	_, err = u.p.Call(ctx, protocol.HostCapability, "ui.command", body)
	return err
}
