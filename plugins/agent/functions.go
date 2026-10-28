package main

import (
	"context"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (a *Agent) turn(ctx context.Context, req *FromWebUI) (*TurnResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.UserInput) == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "user_input is required")
	}

	return &TurnResponse{
		SessionId: req.SessionId,
		Accepted:  true,
		Message:   "agent framework ready",
	}, nil
}
