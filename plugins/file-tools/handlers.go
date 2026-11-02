package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleToolInfo(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	return json.Marshal(p.toolInfo())
}

func (p *plugin) handleRead(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args readArgs
	if err := decodeArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.read(ctx, input, &args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleWrite(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args writeArgs
	if err := decodeArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.write(ctx, input, &args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleGlob(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args globArgs
	if err := decodeArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.glob(ctx, input, &args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleGrep(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args grepArgs
	if err := decodeArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.grep(ctx, input, &args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleEdit(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, err := decodeProviderRequest(req)
	if err != nil {
		return nil, err
	}
	var args editArgs
	if err := decodeArgs(input.Arguments, &args); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.edit(ctx, input, &args)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
