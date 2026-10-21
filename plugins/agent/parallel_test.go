package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type parallelTestSDK struct {
	mu       sync.Mutex
	started  chan string
	release  chan struct{}
	appended []string
	seq      int
}

func (f *parallelTestSDK) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	switch capability + "." + method {
	case "tools.call":
		var in struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(payload, &in)
		f.started <- in.Name
		if in.Name == "first" {
			<-f.release
		}
		return mustJSON(toolResult{Content: in.Name}), nil
	case "session.append":
		var in struct {
			Facts []struct {
				Content string `json:"content"`
			} `json:"facts"`
		}
		_ = json.Unmarshal(payload, &in)
		f.mu.Lock()
		for _, fact := range in.Facts {
			f.appended = append(f.appended, fact.Content)
		}
		seqs := make([]int, len(in.Facts))
		for i := range seqs {
			f.seq++
			seqs[i] = f.seq
		}
		f.mu.Unlock()
		return mustJSON(map[string]any{"seqs": seqs, "next_seq": f.seq + 1}), nil
	default:
		return nil, fmt.Errorf("unexpected call %s.%s", capability, method)
	}
}

func (f *parallelTestSDK) CallWithCallback(context.Context, string, string, json.RawMessage, func(*event)) (json.RawMessage, error) {
	return nil, fmt.Errorf("unexpected callback call")
}

func TestCanRunToolsParallelRequiresAutoAndAllSafe(t *testing.T) {
	p := newPlugin()
	p.cfg.ToolParallelism = "auto"
	p.cfg.MaxParallelTools = 4
	safe := []toolSpec{{Name: "first", ParallelSafe: true}, {Name: "second", ParallelSafe: true}}
	calls := []toolCall{
		{Function: functionCall{Name: "first"}},
		{Function: functionCall{Name: "second"}},
	}
	if !p.canRunToolsParallel(safe, calls) {
		t.Fatal("all-safe batch was not eligible")
	}
	if p.canRunToolsParallel([]toolSpec{{Name: "first", ParallelSafe: true}, {Name: "second"}}, calls) {
		t.Fatal("unsafe batch was eligible")
	}
	p.cfg.ToolParallelism = "serial"
	if p.canRunToolsParallel(safe, calls) {
		t.Fatal("serial mode was eligible")
	}
}

func TestRunToolCallsParallelOverlapsAndKeepsInputOrder(t *testing.T) {
	sdk := &parallelTestSDK{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	p := newPlugin()
	p.sdk = sdk
	p.cfg.ToolParallelism = "auto"
	p.cfg.MaxParallelTools = 2
	st := newTurnState(context.Background(), "s1")
	st.turnID = "t1"
	st.stepID = "t1-s1"
	calls := []toolCall{
		{ID: "call-1", Function: functionCall{Name: "first"}},
		{ID: "call-2", Function: functionCall{Name: "second"}},
	}

	type result struct {
		cancelled bool
		seqs      []int
		err       error
	}
	done := make(chan result, 1)
	go func() {
		cancelled, seqs, err := p.runToolCallsParallel(st, turnIn{SessionID: "s1"}, schemeToolCalling, calls)
		done <- result{cancelled: cancelled, seqs: seqs, err: err}
	}()

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case name := <-sdk.started:
			seen[name] = true
		case <-time.After(time.Second):
			close(sdk.release)
			t.Fatalf("tools did not overlap; seen=%v", seen)
		}
	}
	if !seen["first"] || !seen["second"] {
		close(sdk.release)
		t.Fatalf("seen=%v", seen)
	}
	close(sdk.release)
	res := <-done
	if res.err != nil || res.cancelled || len(res.seqs) != 2 {
		t.Fatalf("result = %#v", res)
	}
	sdk.mu.Lock()
	appended := strings.Join(sdk.appended, ",")
	sdk.mu.Unlock()
	if appended != "first,second" {
		t.Fatalf("appended order = %s", appended)
	}
}
