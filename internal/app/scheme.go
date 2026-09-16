package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tomori/my-go-lite-agent/serve"
)

// applyAgentScheme sets agent defaultScheme via the Config Capability (ADR-0023).
func applyAgentScheme(srv *serve.Server, scheme string) {
	payload, err := srv.CallCommand("agent", "config", "set defaultScheme="+scheme)
	if err != nil {
		fmt.Printf("warn: set scheme %q: %v\n", scheme, err)
		return
	}
	_ = payload
	fmt.Printf("agent scheme: %s\n", scheme)
}

// agentSchemeLabel reads the active scheme for REPL /lp banners.
func agentSchemeLabel(srv *serve.Server) string {
	if srv == nil {
		return ""
	}
	payload, err := srv.CallCommand("agent", "config", "get")
	if err != nil {
		return ""
	}
	var out struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(payload, &out)
	for _, line := range strings.Split(out.Text, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "defaultScheme="); ok {
			return v
		}
	}
	return ""
}
