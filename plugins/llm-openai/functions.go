package main

import (
	"context"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *llmPlugin) complete(
	ctx context.Context,
	req *CompletionRequest,
) (*CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Prompt) == "" && len(req.Messages) == 0 {
		return nil, pluginsdk.ErrCode(
			pluginsdk.CodeBadArguments,
			"prompt or messages is required",
		)
	}

	return &CompletionResponse{
		Model: req.Model,
		Text:  "llm-openai framework ready",
	}, nil
}
