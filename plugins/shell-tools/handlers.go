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

func (p *plugin) handleexecute(
	ctx context.Context,
	req *CallRequest,
) (*CallResponse, error) {
	return p.execute(ctx, req)
}
