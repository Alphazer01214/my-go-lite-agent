package main

import (
	"testing"
	"time"
)

func TestPendingStoreDecideOnce(t *testing.T) {
	store := newPendingStore()
	now := time.Now()
	item := &pendingConfirm{
		Token:     "cf-1",
		CreatedTS: now.UnixMilli(),
		ExpiresTS: now.Add(time.Minute).UnixMilli(),
		Decision:  make(chan string, 1),
	}
	store.add(item)

	if _, err := store.decide("cf-1", ActionAllow, now); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-item.Decision:
		if got != ActionAllow {
			t.Fatalf("decision = %q", got)
		}
	default:
		t.Fatal("decision was not delivered")
	}
	if _, err := store.decide("cf-1", ActionAllow, now); err == nil {
		t.Fatal("second decision must fail")
	}
}

func TestPendingStoreExpires(t *testing.T) {
	store := newPendingStore()
	now := time.Now()
	store.add(&pendingConfirm{
		Token:     "cf-1",
		CreatedTS: now.UnixMilli(),
		ExpiresTS: now.Add(-time.Second).UnixMilli(),
		Decision:  make(chan string, 1),
	})
	if _, err := store.decide("cf-1", ActionAllow, now); err == nil {
		t.Fatal("expired token must fail")
	}
	if len(store.list("", now)) != 0 {
		t.Fatal("expired token must not be listed")
	}
}

func TestApprovalStoreConsumesMatchingTarget(t *testing.T) {
	store := newApprovalStore()
	now := time.Now()
	call := callContext{CallID: "c1", Capability: "file", Method: "write"}
	store.remember(call, now)
	if !store.consume(call, now) {
		t.Fatal("matching call should be consumed")
	}
	if store.consume(call, now) {
		t.Fatal("approval must be one-shot")
	}

	store.remember(call, now)
	other := callContext{CallID: "c1", Capability: "shell", Method: "execute"}
	if store.consume(other, now) {
		t.Fatal("different target must not consume the approval")
	}
}
