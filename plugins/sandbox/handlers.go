package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleCheck(
	ctx context.Context,
	req *pluginsdk.Request,
) (json.RawMessage, error) {
	var input CallRequest
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}

	result, err := p.check(ctx, &input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
