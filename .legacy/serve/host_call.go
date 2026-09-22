package serve

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// CallHost dispatches L0 Host methods for the Medium (/api/call to=host).
// Domain capabilities must not appear here (ADR-0030).
func CallHost(s *Server, method string, payload json.RawMessage) (json.RawMessage, error) {
	if s == nil {
		return nil, fmt.Errorf("no host server")
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	switch method {
	case "plugins":
		return json.Marshal(s.hostPluginsSnapshot())
	case "ensurePlugins":
		var in struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		out, err := s.EnsurePlugins(in.Names)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	case "setPluginEnabled":
		var in struct {
			Name     string `json:"name"`
			Enabled  bool   `json:"enabled"`
			HasEnab  bool   `json:"-"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if in.Name == "" {
			return nil, &protocol.FrameError{Code: "bad_arguments", Message: "name is required"}
		}
		rawMap := map[string]any{}
		_ = json.Unmarshal(payload, &rawMap)
		if v, ok := rawMap["enabled"]; ok {
			b, ok := v.(bool)
			if !ok {
				return nil, &protocol.FrameError{Code: "bad_arguments", Message: "enabled must be boolean"}
			}
			in.Enabled = b
		} else {
			// Missing enabled → treat as disable (safe default).
			in.Enabled = false
		}
		out, err := s.SetPluginEnabled(in.Name, in.Enabled)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	case "pluginSwitch":
		return json.Marshal(map[string]any{"disabled": s.DisabledPluginNames()})
	default:
		return nil, &protocol.FrameError{Code: "method_not_found", Message: "no handler for host." + method}
	}
}
