package app

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/serve"
)

// callPlugin is L0 point-named invocation (ADR-0030). Host does not route by capability.
func callPlugin(srv *serve.Server, plugin, cap, method string, payload any) (json.RawMessage, error) {
	raw := serve.MarshalPayload(payload)
	return srv.CallByPlugin(plugin, cap, method, raw)
}
