package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleStream(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input streamReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}

	result, err := p.stream(ctx, req, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
