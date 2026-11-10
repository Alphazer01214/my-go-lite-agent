package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func runsHandler(t *testing.T) func(string, string, json.RawMessage) (json.RawMessage, error) {
	t.Helper()
	return func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "session.facts":
			return json.RawMessage(`{
				"facts": [
					{"seq": 10, "type": "subagent_spawned", "meta": {"child_session_id": "s-a", "state": "running", "delegation_depth": 1, "started_ts": 1000}},
					{"seq": 11, "type": "subagent_spawned", "meta": {"child_session_id": "s-b", "state": "running", "delegation_depth": 2, "started_ts": 2000}},
					{"seq": 12, "type": "subagent_finished", "meta": {"child_session_id": "s-b", "state": "completed", "duration_ms": 900, "turn_count": 2}},
					{"seq": 13, "type": "subagent_finished", "meta": {"child_session_id": "s-a", "state": "failed", "duration_ms": 5000, "turn_count": 1, "error": "boom"}},
					{"seq": 14, "type": "subagent_spawned", "meta": {"child_session_id": "s-c", "state": "running", "delegation_depth": 1, "started_ts": 9000}}
				],
				"total": 5
			}`), nil
		case "session.info":
			var req map[string]string
			if err := json.Unmarshal(payload, &req); err != nil {
				t.Fatal(err)
			}
			switch req["session_id"] {
			case "s-a":
				return json.RawMessage(`{"session_id":"s-a","title":"subagent: alpha","fact_count":4,"message_count":2}`), nil
			case "s-b":
				return json.RawMessage(`{"session_id":"s-b","title":"subagent: beta","fact_count":6,"message_count":3}`), nil
			case "s-c":
				return json.RawMessage(`{"session_id":"s-c","title":"subagent: gamma","fact_count":1,"message_count":0}`), nil
			default:
				t.Fatalf("unexpected session.info %s", req["session_id"])
				return nil, nil
			}
		default:
			t.Fatalf("unexpected call %s.%s", capability, method)
			return nil, nil
		}
	}
}

func TestListRunsMergesFactsAndEnrichesChildren(t *testing.T) {
	sdk := &fakeSDK{handler: runsHandler(t)}
	p := newPlugin(sdk, config{MaxDepth: 1})

	out, err := p.listRuns(context.Background(), "", "s-parent")
	if err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "s-parent" {
		t.Fatalf("session_id = %q", out.SessionID)
	}
	if len(out.Runs) != 3 {
		t.Fatalf("runs = %d, want 3: %+v", len(out.Runs), out.Runs)
	}

	byID := make(map[string]runView, len(out.Runs))
	for _, run := range out.Runs {
		byID[run.ChildSessionID] = run
	}

	a := byID["s-a"]
	if a.State != "failed" || a.Error != "boom" || a.DurationMs != 5000 || a.TurnCount != 1 {
		t.Fatalf("s-a = %+v", a)
	}
	if a.Title != "subagent: alpha" || a.FactCount != 4 || a.MessageCount != 2 {
		t.Fatalf("s-a enrichment = %+v", a)
	}
	if a.DelegationDepth != 1 || a.StartedTs != 1000 {
		t.Fatalf("s-a start = %+v", a)
	}

	b := byID["s-b"]
	if b.State != "completed" || b.DurationMs != 900 || b.TurnCount != 2 || b.DelegationDepth != 2 {
		t.Fatalf("s-b = %+v", b)
	}

	c := byID["s-c"]
	if c.State != "running" || c.FinishedTs != 0 || c.DelegationDepth != 1 {
		t.Fatalf("s-c = %+v", c)
	}

	// Newest delegation first.
	if out.Runs[0].ChildSessionID != "s-c" || out.Runs[1].ChildSessionID != "s-b" || out.Runs[2].ChildSessionID != "s-a" {
		t.Fatalf("order = %v, %v, %v", out.Runs[0].ChildSessionID, out.Runs[1].ChildSessionID, out.Runs[2].ChildSessionID)
	}
}

func TestListRunsRejectsEmptySession(t *testing.T) {
	called := false
	sdk := &fakeSDK{handler: func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		called = true
		return nil, nil
	}}
	p := newPlugin(sdk, config{MaxDepth: 1})

	_, err := p.listRuns(context.Background(), "", "  ")
	if pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("err = %v", err)
	}
	if called {
		t.Fatal("session.facts must not be called without a session id")
	}
}

func TestListRunsKeepsUnfinishedEntryRunning(t *testing.T) {
	sdk := &fakeSDK{handler: func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "session.facts":
			return json.RawMessage(`{"facts":[{"seq":1,"type":"subagent_spawned","meta":{"child_session_id":"s-x","state":"running","started_ts":1234}}]}`), nil
		case "session.info":
			return nil, pluginsdk.ErrCode("session_not_found", "gone")
		default:
			t.Fatalf("unexpected call %s.%s", capability, method)
			return nil, nil
		}
	}}
	p := newPlugin(sdk, config{MaxDepth: 1})

	out, err := p.listRuns(context.Background(), "", "s-parent")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Runs) != 1 || out.Runs[0].State != "running" || out.Runs[0].StartedTs != 1234 {
		t.Fatalf("runs = %+v", out.Runs)
	}
}
