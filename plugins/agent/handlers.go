package main

import (
	"context"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func handleJSON[Req any, Resp any](
	fn func(context.Context, *Req) (*Resp, error),
) pluginsdk.HandleFunc {
	return pluginsdk.Handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var input Req
		if err := pluginsdk.Decode(req, &input); err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
		return fn(ctx, &input)
	})
}

func (a *Agent) handleTurn(ctx context.Context, req *FromWebUI) (*TurnResponse, error) {
	return a.turn(ctx, req)
}

// func (a *Agent) handleTurnStart(ctx context.Context, req *FromWebUI) (json.RawMessage, error) {

// }
