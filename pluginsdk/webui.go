package pluginsdk

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/protocol"
)

type Layout struct {
}

type WebUI struct {
	p *Plugin
}

// WebUI returns the plugin's UI publisher. Publish targets are always owned
// by the current plugin; mountID supplies only the instance suffix.
func (p *Plugin) WebUI() *WebUI {
	return &WebUI{p: p}
}

// SetState publishes a rebuildable UI state to the current plugin's mount.
func (u *WebUI) SetState(mountID string, state json.RawMessage) error {
	if u == nil || u.p == nil {
		return ErrCode(protocol.CodeServerClosed, "plugin is not initialized")
	}
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
	_, err = u.p.Call(protocol.HostCapability, "ui.state", payload)
	return err
}

// SendCommand publishes a transient UI command to the current plugin's mount.
func (u *WebUI) SendCommand(mountID string, command string, payload json.RawMessage) error {
	if u == nil || u.p == nil {
		return ErrCode(protocol.CodeServerClosed, "plugin is not initialized")
	}
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
	_, err = u.p.Call(protocol.HostCapability, "ui.command", body)
	return err
}
