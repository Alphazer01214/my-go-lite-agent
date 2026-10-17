package main

import (
	"encoding/json"
	"testing"
)

func TestStreamAccumulatorMergesDeltas(t *testing.T) {
	acc := newStreamAccumulator()
	for _, payload := range []string{
		`{"channel":"reasoning","delta":"think"}`,
		`{"channel":"content","delta":"Hel"}`,
		`{"channel":"content","delta":"lo"}`,
		`{"channel":"tool_call","index":0,"id":"call-1","function":{"name":"read"}}`,
		`{"channel":"tool_call","index":0,"function":{"arguments":"{\"path\":\"a\"}"}}`,
	} {
		if !acc.apply(json.RawMessage(payload)) {
			t.Fatalf("chunk not applied: %s", payload)
		}
	}
	got := acc.snapshot("s1", "t1", "st1", "streaming")
	if got.Message.Content != "Hello" || got.Message.Reasoning != "think" {
		t.Fatalf("message = %+v", got.Message)
	}
	if len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].Function.Arguments != `{"path":"a"}` {
		t.Fatalf("tool calls = %+v", got.Message.ToolCalls)
	}
	sequencer := newStreamSequencer()
	if got := sequencer.withNext(got); got.UpdateID != 1 {
		t.Fatalf("update id = %d", got.UpdateID)
	}
}
