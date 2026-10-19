package main

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type fakeAgentSDK struct {
	mu             sync.Mutex
	cancelledCalls []string
	callID         string
	appends        [][]map[string]any
	streams        []streamSnapshot
	nextSeq        int
	block          bool
	started        chan struct{}
	release        chan struct{}
	infoCalls      int
}

func (f *fakeAgentSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch capability + "." + method {
	case "session.info":
		f.infoCalls++
		return mustJSON(sessionInfoOut{Workspace: "E:/workspace"}), nil
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
	case "llm.cancel":
		var in llmCancelIn
		_ = json.Unmarshal(payload, &in)
		f.cancelledCalls = append(f.cancelledCalls, in.CallID)
		return mustJSON(llmCancelOut{CallID: in.CallID, Cancelled: true}), nil
	case "session.stream.update":
		var snapshot streamSnapshot
		_ = json.Unmarshal(payload, &snapshot)
		f.streams = append(f.streams, snapshot)
		return mustJSON(map[string]any{"accepted": true}), nil
	default:
		return nil, errHandler("unexpected call " + capability + "." + method)
	}
}

func (f *fakeAgentSDK) CallWithCallback(ctx context.Context, capability, method string, payload json.RawMessage, callback func(*event)) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if capability != "llm" || method != "complete" {
		return nil, errHandler("unexpected callback call")
	}
	var in llmCompleteIn
	_ = json.Unmarshal(payload, &in)
	f.callID = in.CallID
	callback(&event{Payload: json.RawMessage(`{"channel":"content","delta":"Hi"}`)})
	callback(&event{Payload: json.RawMessage(`{"channel":"reasoning","delta":"plan"}`)})
	if f.block {
		close(f.started)
		select {
		case <-f.release:
			return mustJSON(llmCompleteOut{Content: "late"}), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
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

	out, err := p.turn(context.Background(), turnIn{
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
	if sdk.infoCalls != 1 {
		t.Fatalf("session.info calls = %d", sdk.infoCalls)
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
		out, err := p.turn(context.Background(), turnIn{Input: "hello", SessionID: "s1", MaxSteps: 2})
		if err != nil {
			t.Errorf("turn: %v", err)
		}
		done <- out
	}()

	<-sdk.started
	cancelled, err := p.cancel(context.Background(), cancelIn{SessionID: "s1"})
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
	if len(sdk.cancelledCalls) != 1 || sdk.cancelledCalls[0] == "" {
		t.Fatalf("cancelled LLM calls = %#v", sdk.cancelledCalls)
	}
	if !sawDraft || !sawCancelledEnd {
		t.Fatalf("appends = %#v", sdk.appends)
	}
}

func TestPreCancelledTurnWritesTerminal(t *testing.T) {
	sdk := &fakeAgentSDK{}
	p := newPlugin()
	p.sdk = sdk
	st := newTurnState(context.Background(), "s1")
	st.markCancelled()

	out, err := p.runTurn(st, turnIn{Input: "hello", SessionID: "s1"}, schemeToolCalling, 2)
	if err != nil || !out.Cancelled {
		t.Fatalf("out = %+v, err = %v", out, err)
	}
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	for _, facts := range sdk.appends {
		for _, fact := range facts {
			if fact["type"] != "turn_end" {
				continue
			}
			meta, _ := json.Marshal(fact["meta"])
			var decoded map[string]any
			_ = json.Unmarshal(meta, &decoded)
			if decoded["reason"] == "cancelled" {
				return
			}
		}
	}
	t.Fatalf("appends = %#v", sdk.appends)
}

func TestInboundCancelWritesCancelledTerminal(t *testing.T) {
	sdk := &fakeAgentSDK{
		block:   true,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	defer close(sdk.release)
	p := newPlugin()
	p.sdk = sdk

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var out turnOut
	var err error
	go func() {
		out, err = p.turn(ctx, turnIn{Input: "hello", SessionID: "s1", MaxSteps: 2})
		close(done)
	}()

	<-sdk.started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("turn did not finish after inbound cancel")
	}
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if !out.Cancelled {
		t.Fatalf("out = %+v", out)
	}

	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sawCancelled := false
	sawError := false
	for _, facts := range sdk.appends {
		for _, fact := range facts {
			if fact["type"] != "turn_end" {
				continue
			}
			meta, _ := json.Marshal(fact["meta"])
			var decoded map[string]any
			_ = json.Unmarshal(meta, &decoded)
			switch decoded["reason"] {
			case "cancelled":
				sawCancelled = true
			case "error":
				sawError = true
			}
		}
	}
	if !sawCancelled || sawError {
		t.Fatalf("appends = %#v", sdk.appends)
	}
}

type concurrentAgentSDK struct {
	mu      sync.Mutex
	started chan string
	release chan struct{}
	active  int
	max     int
	nextSeq int
}

func (f *concurrentAgentSDK) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	switch capability + "." + method {
	case "session.info":
		return mustJSON(sessionInfoOut{Workspace: "E:/workspace"}), nil
	case "session.derive":
		return mustJSON(map[string]any{"messages": []any{}}), nil
	case "session.append":
		var in struct {
			Facts []map[string]any `json:"facts"`
		}
		_ = json.Unmarshal(payload, &in)
		f.mu.Lock()
		seqs := make([]int, len(in.Facts))
		for i := range seqs {
			f.nextSeq++
			seqs[i] = f.nextSeq
		}
		next := f.nextSeq + 1
		f.mu.Unlock()
		return mustJSON(map[string]any{"seqs": seqs, "next_seq": next}), nil
	case "session.stream.update":
		return mustJSON(map[string]any{"accepted": true}), nil
	default:
		return nil, errHandler("unexpected call " + capability + "." + method)
	}
}

func (f *concurrentAgentSDK) CallWithCallback(_ context.Context, capability, method string, payload json.RawMessage, _ func(*event)) (json.RawMessage, error) {
	if capability != "llm" || method != "complete" {
		return nil, errHandler("unexpected callback call")
	}
	var in llmCompleteIn
	_ = json.Unmarshal(payload, &in)
	f.mu.Lock()
	f.active++
	if f.active > f.max {
		f.max = f.active
	}
	f.mu.Unlock()
	f.started <- in.CallID
	<-f.release
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	return mustJSON(llmCompleteOut{Content: "done:" + in.CallID, FinishReason: "stop"}), nil
}

func (f *concurrentAgentSDK) EmitWithID(string, string, string, json.RawMessage) error { return nil }

func TestDifferentSessionsRunTurnsConcurrently(t *testing.T) {
	sdk := &concurrentAgentSDK{started: make(chan string, 2), release: make(chan struct{})}
	p := newPlugin()
	p.sdk = sdk

	done := make(chan error, 2)
	for _, sessionID := range []string{"s1", "s2"} {
		sessionID := sessionID
		go func() {
			_, err := p.turn(context.Background(), turnIn{Input: "hello", SessionID: sessionID, MaxSteps: 1})
			done <- err
		}()
	}
	first := <-sdk.started
	second := <-sdk.started
	if first == second {
		t.Fatalf("duplicate call id %q", first)
	}
	sdk.mu.Lock()
	max := sdk.max
	sdk.mu.Unlock()
	if max < 2 {
		t.Fatalf("max concurrent LLM calls = %d", max)
	}
	close(sdk.release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
