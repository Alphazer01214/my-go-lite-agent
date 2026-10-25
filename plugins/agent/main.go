package agent

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type Agent struct {
	sdk       *pluginsdk.Plugin
	workspace string
}

func newAgent() *Agent {
	sdk := pluginsdk.NewPlugin("agent")

	return &Agent{
		sdk: sdk,
	}
}

func main() {
	agent := newAgent()
	var fn pluginsdk.HandleFunc
	fn = func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		return json.RawMessage{}, nil
	}

	agent.sdk.Register("loop", "turn", pluginsdk.NewHandler(fn))
}
