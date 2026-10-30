package main

import (
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleSessionCreate(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := p.consumeSessionCreate(ctx, req); err != nil {
		return nil, err
	}
	return nil, nil
}

func (p *plugin) handleStream(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if err := p.consumeLLMStream(ctx, req); err != nil {
		return nil, err
	}
	return nil, nil
}
