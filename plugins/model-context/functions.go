package main

import "context"

func (p *plugin) build(
	ctx context.Context,
	req *CallRequest,
) (*CallResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &CallResponse{
		Accepted: true,
		Message:  "model-context framework ready",
	}, nil
}
