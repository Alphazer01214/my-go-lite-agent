package main

import (
	"context"
	"encoding/json"
	"testing"
)

type failingAgentSDK struct {
	fakeAgentSDK
}

func (f *failingAgentSDK) CallWithCallback(_ context.Context, capability, method string, payload json.RawMessage, callback func(*event)) (json.RawMessage, error) {
	return nil, errHandler("llm_upstream: http 401")
}

func TestTurnErrorWritesTerminalTurnEnd(t *testing.T) {
	sdk := &failingAgentSDK{}
	p := newPlugin()
	p.sdk = sdk

	out, err := p.turn(context.Background(), turnIn{Input: "hello", SessionID: "s1", MaxSteps: 2})
	if err == nil {
		t.Fatal("turn error = nil")
	}
	if out.SessionID != "s1" {
		t.Fatalf("out = %+v", out)
	}

	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sawTerminal := false
	for _, facts := range sdk.appends {
		for _, fact := range facts {
			if fact["type"] != "turn_end" {
				continue
			}
			meta, _ := json.Marshal(fact["meta"])
			var decoded map[string]any
			_ = json.Unmarshal(meta, &decoded)
			if decoded["reason"] == "error" && decoded["error"] != "" {
				sawTerminal = true
			}
		}
	}
	if !sawTerminal {
		t.Fatalf("appends = %#v", sdk.appends)
	}
}
