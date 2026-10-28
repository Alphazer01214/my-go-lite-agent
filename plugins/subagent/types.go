package main

import "encoding/json"

type CallRequest struct {
	Input json.RawMessage `json:"input,omitempty"`
}

type CallResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
}
