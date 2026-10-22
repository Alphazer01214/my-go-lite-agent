package main

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	sdk sdkCaller
	cfg config

	mu            sync.Mutex
	snapshots     map[string]snapshot
	snapshotOrder []string

	revision atomic.Int64
	seq      atomic.Int64
}

func newPlugin(sdk sdkCaller) *plugin {
	return &plugin{sdk: sdk, cfg: loadConfig(), snapshots: make(map[string]snapshot)}
}

func (p *plugin) call(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if contextID != "" {
		return p.sdk.CallInContext(ctx, contextID, capability, method, payload)
	}
	return p.sdk.Call(ctx, capability, method, payload)
}
