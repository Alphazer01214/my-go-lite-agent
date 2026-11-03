package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleInfo(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	return json.Marshal(p.toolInfo())
}

func (p *plugin) handleExecute(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args executeArgs
	if err := decodeArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.execute(ctx, input, &args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
