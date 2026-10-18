package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type fakeSubagentSDK struct {
	parentDepth int
	started     chan struct{}
	block       bool
}

func (f *fakeSubagentSDK) Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	switch capability + "." + method {
	case "session.info":
		return mustJSON(sessionInfoOut{DelegationDepth: f.parentDepth}), nil
	case "agent.loop.turn":
		if f.block {
			close(f.started)
			<-ctx.Done()
			return nil, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "cancelled")
		}
		return mustJSON(turnOut{SessionID: "child-1", Reply: "done", Steps: 2}), nil
	default:
		return nil, pluginsdk.ErrCode(pluginsdk.CodeMethodNotFound, "unexpected call")
	}
}

func (f *fakeSubagentSDK) CallInContext(ctx context.Context, _, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	return f.Call(ctx, capability, method, payload)
}

func TestRunCreatesChildTask(t *testing.T) {
	sdk := &fakeSubagentSDK{parentDepth: 0}
	p := newPlugin(sdk)
	out, err := p.run(context.Background(), "ctx-1", toolcontract.ProviderInput{Workspace: "E:/project"}, runIn{
		Task: "inspect", ParentSessionID: "parent-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.SessionID != "child-1" || out.Reply != "done" || out.ParentSession != "parent-1" {
		t.Fatalf("out = %+v", out)
	}
	snapshot := p.status(statusIn{TaskID: out.TaskID})
	if snapshot.Status != "completed" || snapshot.DelegationDepth != 1 || snapshot.ChildSessionID != "child-1" {
		t.Fatalf("task = %+v", snapshot)
	}
}

func TestDepthLimit(t *testing.T) {
	p := newPlugin(&fakeSubagentSDK{parentDepth: 2})
	p.cfg.MaxDepth = 2
	_, err := p.run(context.Background(), "", toolcontract.ProviderInput{}, runIn{
		Task: "inspect", ParentSessionID: "parent-1",
	})
	if pluginsdk.Code(err) != "subagent_depth_exceeded" {
		t.Fatalf("error = %v", err)
	}
}

func TestCancelRunningTask(t *testing.T) {
	sdk := &fakeSubagentSDK{block: true, started: make(chan struct{})}
	p := newPlugin(sdk)
	done := make(chan error, 1)
	go func() {
		_, err := p.run(context.Background(), "", toolcontract.ProviderInput{}, runIn{Task: "wait"})
		done <- err
	}()
	<-sdk.started
	tasks := p.list()
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v", tasks)
	}
	result := p.cancel(cancelIn{TaskID: tasks[0].TaskID})
	if !result.Cancelled {
		t.Fatalf("cancel = %+v", result)
	}
	select {
	case err := <-done:
		if pluginsdk.Code(err) != pluginsdk.CodeCallCancelled {
			t.Fatalf("run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not stop")
	}
	if got := p.status(statusIn{TaskID: tasks[0].TaskID}); got.Status != "cancelled" {
		t.Fatalf("status = %+v", got)
	}
}
