package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleList(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	response, err := p.list(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(response)
}

func (p *plugin) handleCall(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input callRequest
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	return p.call(ctx, req, &input)
}
