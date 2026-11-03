package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

type fakeCall struct {
	capability string
	method     string
	payload    json.RawMessage
}

type fakeSDK struct {
	mu      sync.Mutex
	calls   []fakeCall
	handler func(capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

func (f *fakeSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{capability: capability, method: method, payload: payload})
	f.mu.Unlock()
	return f.handler(capability, method, payload)
}

func (f *fakeSDK) callsOf(capability, method string) []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeCall
	for _, call := range f.calls {
		if call.capability == capability && call.method == method {
			out = append(out, call)
		}
	}
	return out
}

func toolCatalogHandler(t *testing.T) func(string, string, json.RawMessage) (json.RawMessage, error) {
	t.Helper()
	return func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "tool.list":
			return json.RawMessage(`{"tools":[
				{"name":"file_write","capability":"file","method":"write"},
				{"name":"shell_execute","capability":"shell","method":"execute"}
			]}`), nil
		case "session.append":
			return json.RawMessage(`{"seqs":[1]}`), nil
		default:
			return nil, fmt.Errorf("unexpected sdk call %s.%s", capability, method)
		}
	}
}

func newTestPlugin(t *testing.T, cfg config) (*plugin, *fakeSDK) {
	t.Helper()
	sdk := &fakeSDK{handler: toolCatalogHandler(t)}
	p := newPlugin(sdk, cfg, t.TempDir()+"/config.json")
	return p, sdk
}
