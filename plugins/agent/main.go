package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type Agent struct {
	sdk       *pluginsdk.Plugin
	workspace string
}

func handle(fn func(ctx context.Context, req *pluginsdk.Request) (interface{}, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		res, err := fn(ctx, req)
		if err != nil {
			return nil, err
		}
		if res == nil {
			return json.RawMessage("null"), nil
		}
		byt, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return byt, nil
	}
}

func decode(src *pluginsdk.Request, dst interface{}) error {
	if len(src.Payload) == 0 {
		return errors.New("payload is required")
	}
	return json.Unmarshal(src.Payload, dst)
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
