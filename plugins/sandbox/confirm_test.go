package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type adjudicateResult struct {
	out json.RawMessage
	err error
}

// startWriteCall runs one file_write adjudication in the background. The call
// blocks on confirmation; the returned channel closes when it returns.
func startWriteCall(t *testing.T, p *plugin, ctx context.Context) (<-chan adjudicateResult, *[]json.RawMessage) {
	t.Helper()
	payload := json.RawMessage(`{"name":"file_write","arguments":{"path":"a.txt"},"session_id":"s-1","call_id":"c-1"}`)
	req := &pluginsdk.Request{Capability: "tools", Method: "call", Payload: payload}
	proceeded := new([]json.RawMessage)
	done := make(chan adjudicateResult, 1)
	go func() {
		out, err := p.adjudicate(ctx, req, seamPayload{
			Name:      "file_write",
			Arguments: json.RawMessage(`{"path":"a.txt"}`),
			SessionID: "s-1",
			CallID:    "c-1",
		}, func(_ context.Context, body json.RawMessage) (json.RawMessage, error) {
			*proceeded = append(*proceeded, body)
			return json.RawMessage(`{"content":"written"}`), nil
		})
		done <- adjudicateResult{out: out, err: err}
	}()
	return done, proceeded
}

func waitForPending(t *testing.T, p *plugin) pendingView {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, item := range p.pending.list("", time.Now()) {
			return item
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("call did not create a pending confirmation")
	return pendingView{}
}

func waitResult(t *testing.T, done <-chan adjudicateResult) adjudicateResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("adjudication did not return")
		return adjudicateResult{}
	}
}

func TestConfirmBlocksThenAllows(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	done, proceeded := startWriteCall(t, p, context.Background())

	item := waitForPending(t, p)
	if item.Tool != "file_write" || item.SessionID != "s-1" || item.Token == "" {
		t.Fatalf("pending = %+v", item)
	}
	if p.pending.count() != 1 {
		t.Fatalf("pending count = %d, want 1", p.pending.count())
	}
	decided, err := p.decideCall(context.Background(), &pluginsdk.Request{
		Payload: mustJSON(map[string]any{"token": item.Token, "decision": "allow"}),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decided.State != "approved" {
		t.Fatalf("decide = %+v", decided)
	}
	result := waitResult(t, done)
	if result.err != nil {
		t.Fatalf("adjudicate: %v", result.err)
	}
	if len(*proceeded) != 1 {
		t.Fatalf("proceeded %d times, want once", len(*proceeded))
	}
	if string(result.out) != `{"content":"written"}` {
		t.Fatalf("result = %s", result.out)
	}
	if p.pending.count() != 0 {
		t.Fatalf("pending count after allow = %d", p.pending.count())
	}
	appends := caller.appends_()
	if len(appends) != 2 {
		t.Fatalf("audit writes = %d, want confirm + allow", len(appends))
	}
	first := auditMetaFrom(t, appends[0])
	second := auditMetaFrom(t, appends[1])
	if first.Decision != ActionConfirm || first.ConfirmToken == "" {
		t.Fatalf("confirm audit = %+v", first)
	}
	if second.Decision != ActionAllow || second.ConfirmToken != first.ConfirmToken {
		t.Fatalf("allow audit = %+v", second)
	}
}

func TestApprovedToolsCallSkipsTheNestedDirectSeam(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	done, _ := startWriteCall(t, p, context.Background())
	item := waitForPending(t, p)
	if _, err := p.decideCall(context.Background(), &pluginsdk.Request{
		Payload: mustJSON(map[string]any{"token": item.Token, "decision": "allow"}),
	}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	result := waitResult(t, done)
	if result.err != nil {
		t.Fatalf("adjudicate: %v", result.err)
	}
	writesBefore := len(caller.appends_())

	// tools now forwards the same call into the direct file.write seam.
	req := &pluginsdk.Request{
		Capability: "file", Method: "write",
		Payload: json.RawMessage(`{"arguments":{"path":"a.txt"},"session_id":"s-1","call_id":"c-1"}`),
	}
	var proceeded bool
	if _, err := p.adjudicate(context.Background(), req, seamPayload{
		Arguments: json.RawMessage(`{"path":"a.txt"}`),
		SessionID: "s-1",
		CallID:    "c-1",
	}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		proceeded = true
		return json.RawMessage(`{"content":"written"}`), nil
	}); err != nil {
		t.Fatalf("nested adjudicate: %v", err)
	}
	if !proceeded {
		t.Fatal("the nested direct seam did not proceed")
	}
	if p.pending.count() != 0 {
		t.Fatalf("nested seam opened a second confirmation: %d", p.pending.count())
	}
	if writes := len(caller.appends_()); writes != writesBefore {
		t.Fatalf("nested seam wrote %d extra audit entries", writes-writesBefore)
	}
}

func TestDirectSeamWithoutAnOuterApprovalStillConfirms(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	req := &pluginsdk.Request{
		Capability: "file", Method: "write",
		Payload: json.RawMessage(`{"arguments":{"path":"a.txt"},"session_id":"s-1","call_id":"direct-1"}`),
	}
	done := make(chan adjudicateResult, 1)
	go func() {
		out, err := p.adjudicate(context.Background(), req, seamPayload{
			Arguments: json.RawMessage(`{"path":"a.txt"}`),
			SessionID: "s-1",
			CallID:    "direct-1",
		}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"content":"written"}`), nil
		})
		done <- adjudicateResult{out: out, err: err}
	}()
	_ = waitForPending(t, p)
	if _, err := p.decideCall(context.Background(), &pluginsdk.Request{
		Payload: mustJSON(map[string]any{"token": p.pending.list("", time.Now())[0].Token, "decision": "deny"}),
	}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	if result := waitResult(t, done); result.err != nil {
		t.Fatalf("adjudicate: %v", result.err)
	}
}

func TestConfirmRejectionReturnsDeniedEnvelope(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	done, proceeded := startWriteCall(t, p, context.Background())
	item := waitForPending(t, p)
	if _, err := p.decideCall(context.Background(), &pluginsdk.Request{
		Payload: mustJSON(map[string]any{"token": item.Token, "decision": "deny"}),
	}); err != nil {
		t.Fatalf("decide: %v", err)
	}
	result := waitResult(t, done)
	if result.err != nil {
		t.Fatalf("adjudicate: %v", result.err)
	}
	if len(*proceeded) != 0 {
		t.Fatal("a rejected call must not proceed")
	}
	var envelope toolResult
	if err := json.Unmarshal(result.out, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.IsError || envelope.Meta["code"] != CodeDenied {
		t.Fatalf("envelope = %+v", envelope)
	}
	if !strings.Contains(envelope.Content, "user rejected") {
		t.Fatalf("content = %q", envelope.Content)
	}
	appends := caller.appends_()
	if len(appends) != 2 {
		t.Fatalf("audit writes = %d, want confirm + deny", len(appends))
	}
	if final := auditMetaFrom(t, appends[1]); final.Decision != ActionDeny {
		t.Fatalf("final audit = %+v", final)
	}
}

func TestConfirmCancellationCleansUp(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	done, _ := startWriteCall(t, p, ctx)
	_ = waitForPending(t, p)
	cancel()
	result := waitResult(t, done)
	if pluginsdk.Code(result.err) != pluginsdk.CodeCallCancelled {
		t.Fatalf("error = %v, want call_cancelled", result.err)
	}
	if p.pending.count() != 0 {
		t.Fatalf("pending count = %d, want 0", p.pending.count())
	}
	appends := caller.appends_()
	if len(appends) != 2 {
		t.Fatalf("audit writes = %d, want confirm + cancelled", len(appends))
	}
	if final := auditMetaFrom(t, appends[1]); final.Decision != "cancelled" {
		t.Fatalf("final audit = %+v", final)
	}
}

func TestConfirmTimeoutReturnsExpiredResult(t *testing.T) {
	caller := &fakeCaller{tools: testTools()}
	p := newTestPlugin(t, caller)
	p.cfg.ConfirmTTLSeconds = 1
	done, proceeded := startWriteCall(t, p, context.Background())
	_ = waitForPending(t, p)
	result := waitResult(t, done)
	if result.err != nil {
		t.Fatalf("adjudicate: %v", result.err)
	}
	if len(*proceeded) != 0 {
		t.Fatal("an expired call must not proceed")
	}
	var envelope toolResult
	if err := json.Unmarshal(result.out, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.IsError || envelope.Meta["code"] != CodeConfirmExpired {
		t.Fatalf("envelope = %+v", envelope)
	}
	if p.pending.count() != 0 {
		t.Fatalf("pending count = %d, want 0", p.pending.count())
	}
	appends := caller.appends_()
	if len(appends) != 2 {
		t.Fatalf("audit writes = %d, want confirm + timeout", len(appends))
	}
	if final := auditMetaFrom(t, appends[1]); final.Decision != ActionDeny {
		t.Fatalf("final audit = %+v", final)
	}
}

func TestPendingDecisionIsSingleUse(t *testing.T) {
	store := newPendingStore()
	item := &pendingConfirm{
		Token: "cf-test", ExpiresTS: time.Now().Add(time.Minute).UnixMilli(),
		Decision: make(chan string, 1),
	}
	store.add(item)
	if _, err := store.decide("cf-test", ActionAllow, time.Now()); err != nil {
		t.Fatalf("first decide: %v", err)
	}
	if _, err := store.decide("cf-test", ActionAllow, time.Now()); pluginsdk.Code(err) != CodeConfirmTokenInvalid {
		t.Fatalf("second decide error = %v", err)
	}
	if len(item.Decision) != 1 || <-item.Decision != ActionAllow {
		t.Fatal("the first decision was not delivered")
	}
}

func TestPendingExpiredDecisionIsRejected(t *testing.T) {
	store := newPendingStore()
	store.add(&pendingConfirm{
		Token: "cf-old", ExpiresTS: time.Now().Add(-time.Second).UnixMilli(),
		Decision: make(chan string, 1),
	})
	if _, err := store.decide("cf-old", ActionAllow, time.Now()); pluginsdk.Code(err) != CodeConfirmExpired {
		t.Fatalf("error = %v, want sandbox_confirm_expired", err)
	}
	if store.count() != 0 {
		t.Fatal("an expired entry was left in the store")
	}
}

func TestPendingListFiltersBySession(t *testing.T) {
	store := newPendingStore()
	expires := time.Now().Add(time.Minute).UnixMilli()
	store.add(&pendingConfirm{Token: "cf-1", SessionID: "s-1", ExpiresTS: expires})
	store.add(&pendingConfirm{Token: "cf-2", SessionID: "s-2", ExpiresTS: expires})
	if list := store.list("s-1", time.Now()); len(list) != 1 || list[0].Token != "cf-1" {
		t.Fatalf("session list = %+v", list)
	}
	if list := store.list("", time.Now()); len(list) != 2 {
		t.Fatalf("all list = %+v", list)
	}
}

func auditMetaFrom(t *testing.T, append fakeAppend) auditMeta {
	t.Helper()
	if len(append.Facts) != 1 {
		t.Fatalf("facts = %+v", append.Facts)
	}
	raw, err := json.Marshal(append.Facts[0]["meta"])
	if err != nil {
		t.Fatal(err)
	}
	var meta auditMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}
