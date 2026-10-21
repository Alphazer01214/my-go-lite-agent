package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type appendRecord struct {
	sessionID string
	facts     []factIn
}

type fakeSDK struct {
	mu      sync.Mutex
	depth   int
	turnErr error
	seq     int
	creates []sessionCreateIn
	appends []appendRecord
	turn    turnIn
}

func (f *fakeSDK) Call(_ context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch capability + "." + method {
	case "session.info":
		return mustJSON(sessionInfoOut{
			SessionID: "s-parent", Workspace: "E:/ws", WorkspaceID: "ws-1",
			Model: "test-model", ContextWindow: 65536, DelegationDepth: f.depth,
		}), nil
	case "session.create":
		var in sessionCreateIn
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		f.creates = append(f.creates, in)
		return mustJSON(sessionCreateOut{SessionID: "s-child"}), nil
	case "session.append":
		var in appendIn
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		f.appends = append(f.appends, appendRecord{sessionID: in.SessionID, facts: in.Facts})
		seqs := make([]int, len(in.Facts))
		for i := range in.Facts {
			f.seq++
			seqs[i] = f.seq
		}
		return mustJSON(appendOut{SessionID: in.SessionID, Seqs: seqs, NextSeq: f.seq + 1}), nil
	case "tools.list":
		return mustJSON(toolsListOut{Tools: []toolEntry{
			{Name: "file_read"}, {Name: "file_write"}, {Name: "subagent"},
		}}), nil
	case "loop.turn":
		if err := json.Unmarshal(payload, &f.turn); err != nil {
			return nil, err
		}
		if f.turnErr != nil {
			return nil, f.turnErr
		}
		return mustJSON(turnOut{
			SessionID: "s-child", Reply: "child answer", Steps: 2,
			Usage: usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}), nil
	default:
		return nil, errors.New("unexpected call " + capability + "." + method)
	}
}

func TestRunCreatesChildAndReturnsAnswerAsToolResult(t *testing.T) {
	sdk := &fakeSDK{}
	p := newPlugin(sdk)
	out, err := p.run(context.Background(), providerInput{SessionID: "s-parent"}, runIn{
		Prompt: "Find the config loader.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError || out.Content != "child answer" {
		t.Fatalf("tool result = %#v", out)
	}
	if out.Meta["child_session_id"] != "s-child" || out.Meta["state"] != "completed" {
		t.Fatalf("meta = %#v", out.Meta)
	}

	sdk.mu.Lock()
	defer sdk.mu.Unlock()
	if len(sdk.creates) != 1 {
		t.Fatalf("creates = %#v", sdk.creates)
	}
	create := sdk.creates[0]
	if create.ParentSessionID != "s-parent" || create.Origin != "subagent" || create.DelegationDepth != 1 {
		t.Fatalf("child session = %#v", create)
	}
	if len(sdk.turn.AllowedTools) != 2 || sdk.turn.AllowedTools[0] != "file_read" || sdk.turn.AllowedTools[1] != "file_write" {
		t.Fatalf("child tools = %#v", sdk.turn.AllowedTools)
	}
	if sdk.turn.Origin != "subagent" || sdk.turn.ParentSessionID != "s-parent" {
		t.Fatalf("child turn = %#v", sdk.turn)
	}

	var spawned, finished int
	for _, record := range sdk.appends {
		for _, fact := range record.facts {
			var meta map[string]any
			_ = json.Unmarshal(fact.Meta, &meta)
			switch fact.Type {
			case "subagent_spawned":
				spawned++
				if meta["child_session_id"] != "s-child" || meta["state"] != "running" {
					t.Fatalf("spawn fact = %#v", meta)
				}
			case "subagent_finished":
				finished++
				if meta["child_session_id"] != "s-child" || meta["state"] != "completed" {
					t.Fatalf("finish fact = %#v", meta)
				}
			}
		}
	}
	if spawned != 1 || finished != 1 {
		t.Fatalf("spawn facts = %d, finish facts = %d", spawned, finished)
	}
}

func TestRunReturnsChildFailureAsToolResult(t *testing.T) {
	sdk := &fakeSDK{turnErr: errors.New("llm down")}
	p := newPlugin(sdk)
	out, err := p.run(context.Background(), providerInput{SessionID: "s-parent"}, runIn{Prompt: "look"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError || out.Content != "Subagent failed: llm down" || out.Meta["state"] != "failed" {
		t.Fatalf("tool result = %#v", out)
	}
}

func TestRunRejectsDepthBeyondLimit(t *testing.T) {
	p := newPlugin(&fakeSDK{depth: 1})
	_, err := p.run(context.Background(), providerInput{SessionID: "s-parent"}, runIn{Prompt: "look"})
	if pluginsdk.Code(err) != codeDepthExceeded {
		t.Fatalf("error = %v, code = %q", err, pluginsdk.Code(err))
	}
}

func TestRunRequiresPrompt(t *testing.T) {
	p := newPlugin(&fakeSDK{})
	_, err := p.run(context.Background(), providerInput{SessionID: "s-parent"}, runIn{})
	if pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("error = %v", err)
	}
}
