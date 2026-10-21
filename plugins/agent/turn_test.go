package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type fakeAgentSDK struct {
	mu             sync.Mutex
	cancelledCalls []string
	callID         string
	completeIn     llmCompleteIn
	calls          []string
	appends        [][]map[string]any
	streams        []streamSnapshot
	nextSeq        int
	block          bool
	started        chan struct{}
	release        chan struct{}
	infoCalls      int
	// outs feeds consecutive llm.complete responses. Empty falls back to the
	// default stop answer.
	outs         []llmCompleteOut
	outIdx       int
	toolErr      error
	catalog      *modelsListOut
	infoOverride *sessionInfoOut
}

func (f *fakeAgentSDK) models() modelsListOut {
	if f.catalog != nil {
		return *f.catalog
	}
	return modelsListOut{
		DefaultModel: "deepseek-chat",
		Models:       []modelConfig{{ID: "deepseek-chat", ContextWindow: 65536}},
	}
}

func (f *fakeAgentSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, capability+"."+method)
	switch capability + "." + method {
	case "session.info":
		f.infoCalls++
		if f.infoOverride != nil {
			return mustJSON(*f.infoOverride), nil
		}
		return mustJSON(sessionInfoOut{
			Workspace: "E:/workspace", Model: "deepseek-chat", ContextWindow: 65536,
		}), nil
	case "llm.models.list":
		return mustJSON(f.models()), nil
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
	case "tools.list":
		return mustJSON(map[string]any{"tools": []toolSpec{{
			Name: "file_write", Description: "write", Risk: 80,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}}}), nil
	case "tools.call":
		if f.toolErr != nil {
			return nil, f.toolErr
		}
		return mustJSON(toolResult{Content: "ok"}), nil
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
	f.mu.Lock()
	f.callID = in.CallID
	f.completeIn = in
	f.calls = append(f.calls, "llm.complete")
	f.mu.Unlock()
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
	if f.outIdx < len(f.outs) {
		out := f.outs[f.outIdx]
		f.mu.Lock()
		f.outIdx++
		f.mu.Unlock()
		return mustJSON(out), nil
	}
	return mustJSON(llmCompleteOut{
		Content: "Hi", Reasoning: "plan", FinishReason: "stop",
		Usage:         usage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3},
		Model:         "deepseek-chat",
		ContextWindow: 65536,
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
				sawUsage = decoded["turn_id"] != nil && decoded["step_id"] != nil && decoded["usage"] != nil &&
					decoded["model"] == "deepseek-chat" && decoded["context_window"] == float64(65536)
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

func TestTurnSendsModelContextMeta(t *testing.T) {
	sdk := &fakeAgentSDK{}
	p := newPlugin()
	p.sdk = sdk

	if _, err := p.turn(context.Background(), turnIn{Input: "hello", SessionID: "s1", MaxSteps: 1}); err != nil {
		t.Fatal(err)
	}
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	if sdk.completeIn.Meta == nil {
		t.Fatal("llm.complete payload has no meta")
	}
	meta := sdk.completeIn.Meta
	if meta.SessionID != "s1" || meta.Workspace != "E:/workspace" || meta.Mode != schemeToolCalling {
		t.Fatalf("meta = %+v", meta)
	}
	if sdk.completeIn.Model != "deepseek-chat" {
		t.Fatalf("complete model = %q", sdk.completeIn.Model)
	}
	if meta.Model != "deepseek-chat" || meta.ContextWindow != 65536 {
		t.Fatalf("model meta = %+v", meta)
	}
	if meta.TurnID == "" || meta.StepID == "" {
		t.Fatalf("meta identity = %+v", meta)
	}
	for _, call := range sdk.calls {
		if call == "model-context.build" {
			t.Fatal("agent still calls model-context.build")
		}
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

	out, err := p.runTurn(st, turnIn{Input: "hello", SessionID: "s1"}, schemeToolCalling, 2, "")
	if err != nil || !out.Cancelled {
		t.Fatalf("out = %+v, err = %v", out, err)
	}
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	sawCancelled := false
	sawNote := false
	for _, facts := range sdk.appends {
		for _, fact := range facts {
			meta, _ := json.Marshal(fact["meta"])
			var decoded map[string]any
			_ = json.Unmarshal(meta, &decoded)
			if fact["type"] == "turn_end" && decoded["reason"] == "cancelled" {
				sawCancelled = true
			}
			if fact["type"] == "message" && decoded["kind"] == "turn_cancelled" &&
				fact["content"] == cancelledTurnNote {
				sawNote = true
			}
		}
	}
	if !sawCancelled || !sawNote {
		t.Fatalf("appends = %#v", sdk.appends)
	}
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

// factsOfType flattens every appended batch and decodes the meta of matching
// facts. It keeps assertions readable across the append-heavy turn loop.
func factsOfType(sdk *fakeAgentSDK, factType string) []map[string]any {
	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	out := make([]map[string]any, 0)
	for _, batch := range sdk.appends {
		for _, fact := range batch {
			if fact["type"] != factType {
				continue
			}
			var meta map[string]any
			raw, _ := json.Marshal(fact["meta"])
			_ = json.Unmarshal(raw, &meta)
			meta["_content"] = fact["content"]
			meta["_role"] = fact["role"]
			out = append(out, meta)
		}
	}
	return out
}

func TestTurnMarksTokenLimitAsTruncatedError(t *testing.T) {
	sdk := &fakeAgentSDK{outs: []llmCompleteOut{{
		Content: "partial answer", Reasoning: "plan", FinishReason: "length",
	}}}
	p := newPlugin()
	p.sdk = sdk

	out, err := p.turn(context.Background(), turnIn{Input: "write a long file", SessionID: "s1", MaxSteps: 4})
	if err == nil || pluginsdk.Code(err) != "llm_output_truncated" {
		t.Fatalf("err = %v, want llm_output_truncated", err)
	}
	if out.Reply != "partial answer" {
		t.Fatalf("reply = %q, want the partial content", out.Reply)
	}
	usage := factsOfType(sdk, "llm_usage")
	if len(usage) != 1 || usage[0]["finish_reason"] != "length" {
		t.Fatalf("llm_usage = %#v", usage)
	}
	stepEnd := factsOfType(sdk, "step_end")
	if len(stepEnd) != 1 || stepEnd[0]["reason"] != "error" || stepEnd[0]["error_code"] != "llm_output_truncated" {
		t.Fatalf("step_end = %#v", stepEnd)
	}
	turnEnd := factsOfType(sdk, "turn_end")
	if len(turnEnd) != 1 || turnEnd[0]["reason"] != "error" ||
		turnEnd[0]["error_code"] != "llm_output_truncated" {
		t.Fatalf("turn_end = %#v", turnEnd)
	}
}

func TestTurnMarksMissingToolCallsAsError(t *testing.T) {
	sdk := &fakeAgentSDK{outs: []llmCompleteOut{{
		Content: "", FinishReason: "tool_calls",
	}}}
	p := newPlugin()
	p.sdk = sdk

	_, err := p.turn(context.Background(), turnIn{Input: "create a file", SessionID: "s1", MaxSteps: 4})
	if err == nil || pluginsdk.Code(err) != "llm_tool_calls_missing" {
		t.Fatalf("err = %v, want llm_tool_calls_missing", err)
	}
	turnEnd := factsOfType(sdk, "turn_end")
	if len(turnEnd) != 1 || turnEnd[0]["reason"] != "error" ||
		turnEnd[0]["error_code"] != "llm_tool_calls_missing" {
		t.Fatalf("turn_end = %#v", turnEnd)
	}
}

func TestTurnCompletesTextAnswerWithZeroToolCalls(t *testing.T) {
	sdk := &fakeAgentSDK{outs: []llmCompleteOut{{
		Content: "done", FinishReason: "stop",
	}}}
	p := newPlugin()
	p.sdk = sdk

	if _, err := p.turn(context.Background(), turnIn{Input: "hi", SessionID: "s1", MaxSteps: 4}); err != nil {
		t.Fatal(err)
	}
	stepEnd := factsOfType(sdk, "step_end")
	if len(stepEnd) != 1 || stepEnd[0]["reason"] != "completed" ||
		stepEnd[0]["tool_calls"] != float64(0) || stepEnd[0]["finish_reason"] != "stop" {
		t.Fatalf("step_end = %#v", stepEnd)
	}
	turnEnd := factsOfType(sdk, "turn_end")
	if len(turnEnd) != 1 || turnEnd[0]["reason"] != "completed" ||
		turnEnd[0]["tool_calls"] != float64(0) || turnEnd[0]["steps"] != float64(1) {
		t.Fatalf("turn_end = %#v", turnEnd)
	}
}

func TestToolErrorExposesStableCode(t *testing.T) {
	sdk := &fakeAgentSDK{
		outs: []llmCompleteOut{
			{
				FinishReason: "tool_calls",
				ToolCalls: []toolCall{{
					ID: "call-1", Type: "function",
					Function: functionCall{Name: "file_write", Arguments: `{"path":"missing/a.txt","content":"x"}`},
				}},
			},
			{Content: "gave up", FinishReason: "stop"},
		},
		toolErr: pluginsdk.ErrCode("path_not_found",
			`parent directory "missing" does not exist; pass create_dirs=true or create it first`),
	}
	p := newPlugin()
	p.sdk = sdk

	if _, err := p.turn(context.Background(), turnIn{Input: "write", SessionID: "s1", MaxSteps: 4}); err != nil {
		t.Fatal(err)
	}
	tools := factsOfType(sdk, "message")
	var toolFact map[string]any
	for _, fact := range tools {
		if fact["_role"] == "tool" {
			toolFact = fact
		}
	}
	if toolFact == nil {
		t.Fatalf("no tool fact: %#v", tools)
	}
	content, _ := toolFact["_content"].(string)
	if !strings.Contains(content, "tool error [path_not_found]:") {
		t.Fatalf("tool content = %q, want the stable code", content)
	}
	if toolFact["error_code"] != "path_not_found" || toolFact["is_error"] != true {
		t.Fatalf("tool meta = %#v", toolFact)
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
	case "llm.models.list":
		return mustJSON(modelsListOut{
			DefaultModel: "deepseek-chat",
			Models:       []modelConfig{{ID: "deepseek-chat", ContextWindow: 65536}},
		}), nil
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

func TestTurnPersistsToolCallsWithAssistantFact(t *testing.T) {
	sdk := &fakeAgentSDK{outs: []llmCompleteOut{
		{
			FinishReason: "tool_calls",
			ToolCalls: []toolCall{{
				ID: "call-1", Type: "function",
				Function: functionCall{Name: "file_write", Arguments: `{"path":"a.txt","content":"x"}`},
			}},
		},
		{Content: "done", FinishReason: "stop"},
	}}
	p := newPlugin()
	p.sdk = sdk

	if _, err := p.turn(context.Background(), turnIn{Input: "write", SessionID: "s1", MaxSteps: 4}); err != nil {
		t.Fatal(err)
	}

	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	var assistantCalls []toolCall
	toolResults := 0
	for _, batch := range sdk.appends {
		for _, fact := range batch {
			if fact["type"] != "message" {
				continue
			}
			switch fact["role"] {
			case "assistant":
				raw, _ := json.Marshal(fact["tool_calls"])
				var calls []toolCall
				if err := json.Unmarshal(raw, &calls); err != nil {
					t.Fatalf("assistant tool_calls = %s, err = %v", raw, err)
				}
				if len(calls) > 0 {
					assistantCalls = calls
				}
			case "tool":
				toolResults++
			}
		}
	}
	if len(assistantCalls) != 1 || assistantCalls[0].ID != "call-1" ||
		assistantCalls[0].Function.Name != "file_write" {
		t.Fatalf("assistant tool_calls = %+v", assistantCalls)
	}
	if toolResults != 1 {
		t.Fatalf("tool results = %d, want 1", toolResults)
	}
}

func TestTurnFallsBackWhenSessionModelLeftCatalog(t *testing.T) {
	sdk := &fakeAgentSDK{
		infoOverride: &sessionInfoOut{
			Workspace: "E:/workspace", Model: "mimo-v2.6-flash", ContextWindow: 1000000,
		},
		catalog: &modelsListOut{
			DefaultModel: "deepseek-flash",
			Models: []modelConfig{
				{ID: "deepseek-flash", ContextWindow: 1000000},
				{ID: "deepseek-v4-pro", ContextWindow: 128000},
			},
		},
	}
	p := newPlugin()
	p.sdk = sdk

	if _, err := p.turn(context.Background(), turnIn{Input: "hi", SessionID: "s1", MaxSteps: 1}); err != nil {
		t.Fatal(err)
	}
	sdk.mu.Lock()
	model := sdk.completeIn.Model
	sdk.mu.Unlock()
	if model != "deepseek-flash" {
		t.Fatalf("complete model = %q, want deepseek-flash", model)
	}
	headers := factsOfType(sdk, "request_header")
	if len(headers) != 1 || headers[0]["model"] != "deepseek-flash" ||
		headers[0]["model_fallback_from"] != "mimo-v2.6-flash" {
		t.Fatalf("request_header = %#v", headers)
	}
}

type blockingToolSDK struct {
	*fakeAgentSDK
	started chan struct{}
	release chan struct{}
}

func (f *blockingToolSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if capability == "tools" && method == "call" {
		close(f.started)
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return mustJSON(toolResult{Content: "ok"}), nil
	}
	return f.fakeAgentSDK.Call(ctx, capability, method, payload)
}

func TestCancelledToolStepClosesEveryToolCallID(t *testing.T) {
	base := &fakeAgentSDK{outs: []llmCompleteOut{{
		FinishReason: "tool_calls",
		ToolCalls: []toolCall{
			{ID: "c1", Type: "function", Function: functionCall{Name: "file_write", Arguments: `{}`}},
			{ID: "c2", Type: "function", Function: functionCall{Name: "file_write", Arguments: `{}`}},
		},
	}}}
	sdk := &blockingToolSDK{fakeAgentSDK: base, started: make(chan struct{}), release: make(chan struct{})}
	p := newPlugin()
	p.sdk = sdk

	done := make(chan turnOut, 1)
	go func() {
		out, err := p.turn(context.Background(), turnIn{Input: "write twice", SessionID: "s1", MaxSteps: 2})
		if err != nil {
			t.Errorf("turn: %v", err)
		}
		done <- out
	}()
	<-sdk.started
	if _, err := p.cancel(context.Background(), cancelIn{SessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	close(sdk.release)
	out := <-done
	if !out.Cancelled {
		t.Fatalf("out = %+v", out)
	}

	base.mu.Lock()
	defer base.mu.Unlock()
	seen := map[string]map[string]any{}
	for _, batch := range base.appends {
		for _, fact := range batch {
			if fact["type"] == "message" && fact["role"] == "tool" {
				meta, _ := json.Marshal(fact["meta"])
				var decoded map[string]any
				_ = json.Unmarshal(meta, &decoded)
				decoded["_content"] = fact["content"]
				seen[fact["tool_call_id"].(string)] = decoded
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("tool results = %#v", seen)
	}
	if seen["c1"]["_content"] != "ok" {
		t.Fatalf("executed tool result = %#v", seen["c1"])
	}
	if seen["c2"]["cancelled"] != true || seen["c2"]["_content"] != cancelledToolResult {
		t.Fatalf("synthetic tool result = %#v", seen["c2"])
	}
}
