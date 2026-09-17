package app

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/serve"
)

// callPlugin is L0 point-named invocation (ADR-0030). Host does not route by capability.
func callPlugin(srv *serve.Server, plugin, cap, method string, payload any) (json.RawMessage, error) {
	raw := serve.MarshalPayload(payload)
	return srv.CallByPlugin(plugin, cap, method, raw)
}

// runLoopTurn invokes the agent plugin's loop.turn. Host no longer owns
// RunTurn / per-session locks (ADR-0030; locks live in the loop plugin).
func runLoopTurn(srv *serve.Server, sessionID, input string) (*pluginsdk.TurnResult, error) {
	payload := map[string]any{"input": input, "allowSubagent": true}
	if sessionID != "" {
		payload["sessionId"] = sessionID
	}
	raw, err := callPlugin(srv, "agent", "loop", "turn", payload)
	if err != nil {
		return nil, err
	}
	var out pluginsdk.TurnResult
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("loop.turn: bad payload: %w", err)
		}
	}
	if out.User == "" {
		out.User = input
	}
	return &out, nil
}

// cancelLoopTurn asks the agent plugin to stop an in-flight turn.
func cancelLoopTurn(srv *serve.Server, sessionID string) {
	_, _ = callPlugin(srv, "agent", "loop", "cancel", map[string]any{"sessionId": sessionID})
}
