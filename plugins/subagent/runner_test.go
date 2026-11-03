package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type fakeCall struct {
	contextID  string
	capability string
	method     string
	payload    json.RawMessage
}

type fakeSDK struct {
	calls   []fakeCall
	handler func(capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

func (f *fakeSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	return f.CallInContext(ctx, "", capability, method, payload)
}

func (f *fakeSDK) CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	f.calls = append(f.calls, fakeCall{contextID: contextID, capability: capability, method: method, payload: payload})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.handler(capability, method, payload)
}

func (f *fakeSDK) callsOf(capability, method string) []fakeCall {
	var out []fakeCall
	for _, call := range f.calls {
		if call.capability == capability && call.method == method {
			out = append(out, call)
		}
	}
	return out
}

func rootSession() json.RawMessage {
	return json.RawMessage(`{"session_id":"s-parent","workspace":"E:/ws","origin":"agent","model":"m1"}`)
}

func childSession() json.RawMessage {
	return json.RawMessage(`{"session_id":"s-child","workspace":"E:/ws","parent_session_id":"s-parent","origin":"subagent","model":"m1"}`)
}

func successHandler(t *testing.T) func(string, string, json.RawMessage) (json.RawMessage, error) {
	t.Helper()
	return func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "session.info":
			return rootSession(), nil
		case "session.create":
			return childSession(), nil
		case "session.append":
			return json.RawMessage(`{"seqs":[1]}`), nil
		case "loop.turn":
			return json.RawMessage(`{"session_id":"s-child","model":"m1","messages":[{"role":"assistant","text":"done"}],"seqs":[1]}`), nil
		default:
			t.Fatalf("unexpected call %s.%s", capability, method)
			return nil, nil
		}
	}
}

func TestRunCreatesChildSessionAndReturnsReply(t *testing.T) {
	sdk := &fakeSDK{handler: successHandler(t)}
	p := newPlugin(sdk, config{MaxDepth: 1})

	result, err := p.run(context.Background(), "", providerInput{SessionID: "s-parent", Workspace: "E:/ws"}, runIn{Prompt: "  do the task  "})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("is_error = true: %s", result.Content)
	}
	if result.Content != "done" {
		t.Fatalf("content = %q", result.Content)
	}
	if result.Meta["child_session_id"] != "s-child" {
		t.Fatalf("child meta = %#v", result.Meta)
	}
	if result.Meta["turn_count"] != 1 {
		t.Fatalf("turn_count = %#v", result.Meta["turn_count"])
	}

	creates := sdk.callsOf("session", "create")
	if len(creates) != 1 {
		t.Fatalf("session.create calls = %d", len(creates))
	}
	var create sessionCreateReq
	if err := json.Unmarshal(creates[0].payload, &create); err != nil {
		t.Fatal(err)
	}
	if create.ParentSessionID != "s-parent" || create.Origin != "subagent" {
		t.Fatalf("create = %+v", create)
	}
	if create.Workspace != "E:/ws" || create.Model != "m1" {
		t.Fatalf("create inheritance = %+v", create)
	}
	if !strings.HasPrefix(create.Title, "subagent: ") {
		t.Fatalf("title = %q", create.Title)
	}

	turns := sdk.callsOf("loop", "turn")
	if len(turns) != 1 {
		t.Fatalf("loop.turn calls = %d", len(turns))
	}
	var turn turnReq
	if err := json.Unmarshal(turns[0].payload, &turn); err != nil {
		t.Fatal(err)
	}
	if turn.SessionID != "s-child" || turn.UserInput != "do the task" {
		t.Fatalf("turn = %+v", turn)
	}
	if turn.Workspace != "E:/ws" || turn.Model != "m1" {
		t.Fatalf("turn inheritance = %+v", turn)
	}

	appends := sdk.callsOf("session", "append")
	if len(appends) != 2 {
		t.Fatalf("append calls = %d", len(appends))
	}
	if got := factTypes(t, appends); got != "subagent_spawned,subagent_finished" {
		t.Fatalf("fact types = %s", got)
	}
}

func factTypes(t *testing.T, calls []fakeCall) string {
	t.Helper()
	var types []string
	for _, call := range calls {
		var req appendReq
		if err := json.Unmarshal(call.payload, &req); err != nil {
			t.Fatal(err)
		}
		for _, fact := range req.Facts {
			types = append(types, fact.Type)
		}
	}
	return strings.Join(types, ",")
}

func TestRunRejectsDepthExceeded(t *testing.T) {
	sdk := &fakeSDK{handler: func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "session.info":
			var req map[string]string
			if err := json.Unmarshal(payload, &req); err != nil {
				t.Fatal(err)
			}
			if req["session_id"] == "s-parent" {
				return json.RawMessage(`{"session_id":"s-parent","workspace":"E:/ws","parent_session_id":"s-root","origin":"subagent","model":"m1"}`), nil
			}
			return json.RawMessage(`{"session_id":"s-root","workspace":"E:/ws","origin":"agent","model":"m1"}`), nil
		default:
			t.Fatalf("unexpected call %s.%s", capability, method)
			return nil, nil
		}
	}}
	p := newPlugin(sdk, config{MaxDepth: 1})

	_, err := p.run(context.Background(), "", providerInput{SessionID: "s-parent"}, runIn{Prompt: "nested"})
	if pluginsdk.Code(err) != codeDepthExceeded {
		t.Fatalf("err = %v, code = %s", err, pluginsdk.Code(err))
	}
	if len(sdk.callsOf("session", "create")) != 0 {
		t.Fatal("child session must not be created after depth rejection")
	}
}

func TestRunPropagatesCancellation(t *testing.T) {
	sdk := &fakeSDK{handler: func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "session.info":
			return rootSession(), nil
		case "session.create":
			return childSession(), nil
		case "session.append":
			return json.RawMessage(`{"seqs":[1]}`), nil
		case "loop.turn":
			return nil, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "call cancelled")
		default:
			t.Fatalf("unexpected call %s.%s", capability, method)
			return nil, nil
		}
	}}
	p := newPlugin(sdk, config{MaxDepth: 1})

	_, err := p.run(context.Background(), "", providerInput{SessionID: "s-parent"}, runIn{Prompt: "cancel me"})
	if pluginsdk.Code(err) != pluginsdk.CodeCallCancelled {
		t.Fatalf("err = %v, code = %s", err, pluginsdk.Code(err))
	}

	var finishedState string
	for _, call := range sdk.callsOf("session", "append") {
		var req appendReq
		if err := json.Unmarshal(call.payload, &req); err != nil {
			t.Fatal(err)
		}
		for _, fact := range req.Facts {
			if fact.Type != factTypeFinished {
				continue
			}
			var meta map[string]any
			if err := json.Unmarshal(fact.Meta, &meta); err != nil {
				t.Fatal(err)
			}
			finishedState, _ = meta["state"].(string)
		}
	}
	if finishedState != "cancelled" {
		t.Fatalf("finished state = %q", finishedState)
	}
}

func TestRunMarksEmptyReplyAsError(t *testing.T) {
	sdk := &fakeSDK{handler: func(capability, method string, payload json.RawMessage) (json.RawMessage, error) {
		switch capability + "." + method {
		case "session.info":
			return rootSession(), nil
		case "session.create":
			return childSession(), nil
		case "session.append":
			return json.RawMessage(`{"seqs":[1]}`), nil
		case "loop.turn":
			return json.RawMessage(`{"session_id":"s-child","messages":[{"role":"assistant","text":""}]}`), nil
		default:
			t.Fatalf("unexpected call %s.%s", capability, method)
			return nil, nil
		}
	}}
	p := newPlugin(sdk, config{MaxDepth: 1})

	result, err := p.run(context.Background(), "", providerInput{SessionID: "s-parent"}, runIn{Prompt: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("empty reply must be an error result")
	}
	if result.Content != "Subagent returned no text." {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestFinalReplyPicksLastAssistantText(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleAssistant, Text: "first"},
		{Role: pluginsdk.RoleTool, Text: "tool output"},
		{Role: pluginsdk.RoleAssistant, Text: ""},
		{Role: pluginsdk.RoleAssistant, Text: "final"},
	}
	if got := finalReply(messages); got != "final" {
		t.Fatalf("finalReply = %q", got)
	}
}
