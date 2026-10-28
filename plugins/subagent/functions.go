package main

import "context"

func (p *plugin) run(
	ctx context.Context,
	req *CallRequest,
) (*CallResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &CallResponse{
		Accepted: true,
		Message:  "subagent framework ready",
	}, nil
}
