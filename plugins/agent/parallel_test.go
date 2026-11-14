package main

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func testToolCall(id, name string) pluginsdk.ToolCall {
	return pluginsdk.ToolCall{
		ID:   id,
		Type: "function",
		FunctionCall: pluginsdk.FunctionCall{
			Name:      name,
			Arguments: []byte(`{}`),
		},
	}
}

func testToolFact(id string) sessionFact {
	return sessionFact{
		Type:        factTypeMessage,
		Role:        pluginsdk.RoleTool,
		TextContent: id,
		ToolCallID:  id,
	}
}

func TestPlanToolBatchesKeepsBarrierOrder(t *testing.T) {
	safe := map[string]bool{"file_read": true, "file_glob": true, "subagent": true}
	calls := []pluginsdk.ToolCall{
		testToolCall("read-a", "file_read"),
		testToolCall("read-b", "file_glob"),
		testToolCall("write-c", "file_write"),
		testToolCall("sub-d", "subagent"),
		testToolCall("sub-e", "subagent"),
		testToolCall("shell-f", "shell_execute"),
	}
	got := planToolBatches(calls, safe)
	want := [][]string{
		{"read-a", "read-b"},
		{"write-c"},
		{"sub-d", "sub-e"},
		{"shell-f"},
	}
	if len(got) != len(want) {
		t.Fatalf("batches = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		ids := make([]string, 0, len(got[i]))
		for _, call := range got[i] {
			ids = append(ids, call.ID)
		}
		if !reflect.DeepEqual(ids, want[i]) {
			t.Fatalf("batch %d = %v, want %v", i, ids, want[i])
		}
	}
}

func TestRunToolBatchBoundedAndOrdered(t *testing.T) {
	calls := []pluginsdk.ToolCall{
		testToolCall("a", "subagent"),
		testToolCall("b", "subagent"),
		testToolCall("c", "subagent"),
	}
	started := make(chan string, len(calls))
	release := make(chan struct{})
	var current, peak atomic.Int64

	execute := func(ctx context.Context, call pluginsdk.ToolCall) (sessionFact, error) {
		value := current.Add(1)
		for {
			seen := peak.Load()
			if value <= seen || peak.CompareAndSwap(seen, value) {
				break
			}
		}
		started <- call.ID
		select {
		case <-release:
		case <-ctx.Done():
			current.Add(-1)
			return sessionFact{}, ctx.Err()
		}
		current.Add(-1)
		return testToolFact(call.ID), nil
	}

	done := make(chan struct{})
	var facts []sessionFact
	var err error
	go func() {
		facts, err = runToolBatch(context.Background(), calls, 2, execute)
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for parallel calls")
		}
	}
	select {
	case id := <-started:
		t.Fatalf("third call %s started before a slot was free", id)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for batch")
	}
	if err != nil {
		t.Fatalf("runToolBatch: %v", err)
	}
	if peak.Load() > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", peak.Load())
	}
	for i, fact := range facts {
		if fact.ToolCallID != calls[i].ID {
			t.Fatalf("fact %d = %q, want %q", i, fact.ToolCallID, calls[i].ID)
		}
	}
}

func TestRunToolBatchKeepsResultOrder(t *testing.T) {
	calls := []pluginsdk.ToolCall{
		testToolCall("slow", "subagent"),
		testToolCall("fast", "subagent"),
	}
	delays := map[string]time.Duration{"slow": 40 * time.Millisecond, "fast": 5 * time.Millisecond}
	execute := func(_ context.Context, call pluginsdk.ToolCall) (sessionFact, error) {
		time.Sleep(delays[call.ID])
		return testToolFact(call.ID), nil
	}
	facts, err := runToolBatch(context.Background(), calls, 2, execute)
	if err != nil {
		t.Fatalf("runToolBatch: %v", err)
	}
	if len(facts) != 2 || facts[0].ToolCallID != "slow" || facts[1].ToolCallID != "fast" {
		t.Fatalf("facts = %+v, want original order", facts)
	}
}

func TestRunToolBatchCancelsSiblingsOnFatalError(t *testing.T) {
	calls := []pluginsdk.ToolCall{
		testToolCall("fail", "subagent"),
		testToolCall("blocked", "subagent"),
	}
	execute := func(ctx context.Context, call pluginsdk.ToolCall) (sessionFact, error) {
		if call.ID == "fail" {
			return sessionFact{}, errors.New("boom")
		}
		<-ctx.Done()
		return sessionFact{}, ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := runToolBatch(context.Background(), calls, 2, execute)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error")
		}
	case <-time.After(time.Second):
		t.Fatal("batch did not stop siblings after a fatal error")
	}
}

func TestNormalizeToolCallsRejectsDuplicateIDs(t *testing.T) {
	messages := []pluginsdk.ChatMessage{{
		Role: pluginsdk.RoleAssistant,
		ToolCalls: []pluginsdk.ToolCall{
			testToolCall("same", "file_read"),
			testToolCall("same", "file_read"),
		},
	}}
	err := normalizeToolCalls(messages, "turn", 1)
	if err == nil || pluginsdk.Code(err) != codeToolCallInvalid {
		t.Fatalf("err = %v, want %s", err, codeToolCallInvalid)
	}
}

func TestDefaultConfigParallelizesSubagent(t *testing.T) {
	cfg := defaultConfig()
	if !cfg.parallelEnabled() {
		t.Fatal("default config should enable parallel tool calls")
	}
	if !cfg.safeToolSet()["subagent"] {
		t.Fatal("subagent must be in the default parallel-safe set")
	}
	if cfg.safeToolSet()["shell_execute"] || cfg.safeToolSet()["file_write"] {
		t.Fatal("mutating tools must not be parallel-safe by default")
	}
}
