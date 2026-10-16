package main

import (
	"encoding/json"
	"sync"
	"testing"
)

type fakeAgentSDK struct {
	mu      sync.Mutex
	appends [][]map[string]any
	streams []streamSnapshot
	nextSeq int
	block   bool
	started chan struct{}
	release chan struct{}
}

func (f *fakeAgentSDK) Call(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch capability + "." + method {
	case "session.derive":
		return mustJSON(map[string]any{"messages": []any{}}), nil
	case "session.append":
		var in struct {
			Facts []map[string]any `json:"facts"`
		}
		_ = json.Unmarshal(payload, &in)
		f.appends = append(f.appends, in.Facts)
		seqs := make([]int, 0, len(in.Facts))
		for range in.Facts {
			f.nextSeq++
			seqs = append(seqs, f.nextSeq)
		}
		return mustJSON(map[string]any{"seqs": seqs, "next_seq": f.nextSeq + 1}), nil
	case "session.stream.update":
		var snapshot streamSnapshot
		_ = json.Unmarshal(payload, &snapshot)
		f.streams = append(f.streams, snapshot)
		return mustJSON(map[string]any{"accepted": true}), nil
	default:
		return nil, errHandler("unexpected call " + capability + "." + method)
	}
}

func (f *fakeAgentSDK) CallWithCallback(capability, method string, payload json.RawMessage, callback func(*event)) (json.RawMessage, error) {
	if capability != "llm" || method != "complete" {
		return nil, errHandler("unexpected callback call")
	}
	callback(&event{Payload: json.RawMessage(`{"channel":"content","delta":"Hi"}`)})
	callback(&event{Payload: json.RawMessage(`{"channel":"reasoning","delta":"plan"}`)})
	if f.block {
		close(f.started)
		<-f.release
		return mustJSON(llmCompleteOut{Content: "late"}), nil
	}
	return mustJSON(llmCompleteOut{
		Content: "Hi", Reasoning: "plan", FinishReason: "stop",
		Usage: usage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3},
	}), nil
}

func (f *fakeAgentSDK) EmitWithID(id, capability, method string, payload json.RawMessage) error {
	return nil
}

func TestTurnPersistsMetadataAndPublishesStreamSnapshots(t *testing.T) {
	sdk := &fakeAgentSDK{}
	p := newPlugin()
	p.sdk = sdk

	out, err := p.turn(turnIn{
		Input:     "hello",
		SessionID: "s1",
		MaxSteps:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "s1" || out.Reply != "Hi" || out.Steps != 1 {
		t.Fatalf("out = %+v", out)
	}

	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	if len(sdk.streams) < 2 {
		t.Fatalf("streams = %+v", sdk.streams)
	}
	sawContent := false
	sawComplete := false
	for _, snapshot := range sdk.streams {
		if snapshot.SessionID != "s1" || snapshot.TurnID == "" || snapshot.StepID == "" {
			t.Fatalf("snapshot identity = %+v", snapshot)
		}
		if snapshot.Status == "streaming" && snapshot.Message.Content == "Hi" {
			sawContent = true
		}
		if snapshot.Status == "complete" && snapshot.Message.Reasoning == "plan" {
			sawComplete = true
		}
	}
	if !sawContent || !sawComplete {
		t.Fatalf("streams = %+v", sdk.streams)
	}

	sawUsage := false
	sawAssistantMeta := false
	for _, facts := range sdk.appends {
		for _, fact := range facts {
			meta, _ := json.Marshal(fact["meta"])
			var decoded map[string]any
			_ = json.Unmarshal(meta, &decoded)
			if fact["type"] == "llm_usage" {
				sawUsage = decoded["turn_id"] != nil && decoded["step_id"] != nil && decoded["usage"] != nil
			}
			if fact["type"] == "message" && fact["role"] == "assistant" {
				sawAssistantMeta = decoded["turn_id"] != nil && decoded["step_id"] != nil && decoded["reasoning"] == "plan"
			}
		}
	}
	if !sawUsage || !sawAssistantMeta {
		t.Fatalf("appends = %#v", sdk.appends)
	}
}

func TestCancelSavesAccumulatedAssistantDraft(t *testing.T) {
	sdk := &fakeAgentSDK{
		block:   true,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(sdk.release)
	p := newPlugin()
	p.sdk = sdk

	done := make(chan turnOut, 1)
	go func() {
		out, err := p.turn(turnIn{Input: "hello", SessionID: "s1", MaxSteps: 2})
		if err != nil {
			t.Errorf("turn: %v", err)
		}
		done <- out
	}()

	<-sdk.started
	cancelled, err := p.cancel(cancelIn{SessionID: "s1"})
	if err != nil || !cancelled.Cancelled {
		t.Fatalf("cancel = %+v, %v", cancelled, err)
	}
	out := <-done
	if !out.Cancelled || out.Reply != "Hi" {
		t.Fatalf("out = %+v", out)
	}

	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sawDraft := false
	sawCancelledEnd := false
	for _, facts := range sdk.appends {
		for _, fact := range facts {
			if fact["type"] == "message" && fact["role"] == "assistant" && fact["content"] == "Hi" {
				sawDraft = true
			}
			if fact["type"] == "turn_end" {
				meta, _ := json.Marshal(fact["meta"])
				if string(meta) == "" {
					continue
				}
				var decoded map[string]any
				_ = json.Unmarshal(meta, &decoded)
				if decoded["reason"] == "cancelled" {
					sawCancelledEnd = true
				}
			}
		}
	}
	if !sawDraft || !sawCancelledEnd {
		t.Fatalf("appends = %#v", sdk.appends)
	}
}
