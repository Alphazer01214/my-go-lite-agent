package main

import "context"

func (p *plugin) read(
	ctx context.Context,
	req *CallRequest,
) (*CallResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &CallResponse{
		Accepted: true,
		Message:  "file-tools framework ready",
	}, nil
}
