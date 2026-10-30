package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleTurn(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input fromWebUI
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}

	result, err := p.turn(ctx, req, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleCancel(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input cancelReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.cancel(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
