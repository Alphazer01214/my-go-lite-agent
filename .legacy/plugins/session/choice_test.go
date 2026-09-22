package main

import (
	"strings"
	"testing"
	"time"
)

// TestChoiceCenterResolveFeedsWaitingAsk: choice.ask waits on the resolve
// channel and the first responder wins (ADR-0034).
func TestChoiceCenterResolveFeedsWaitingAsk(t *testing.T) {
	c := newChoiceCenter()
	id, ch := c.register()
	if !strings.HasPrefix(id, "choice-") {
		t.Fatalf("want choice-<n> id, got %q", id)
	}
	if !c.resolve(id, "allow") {
		t.Fatal("first resolve must win")
	}
	select {
	case v := <-ch:
		if v != "allow" {
			t.Fatalf("want allow, got %q", v)
		}
	case <-time.After(time.Second):
		t.Fatal("ask was not unblocked by respond")
	}
	if c.resolve(id, "deny") {
		t.Fatal("second resolve must be rejected (already resolved)")
	}
}

// TestChoiceCenterDropRetires: a timed-out ask can no longer be resolved, so
// a late Medium answer can never rule after the fail-closed deadline.
func TestChoiceCenterDropRetires(t *testing.T) {
	c := newChoiceCenter()
	id, _ := c.register()
	c.drop(id)
	if c.resolve(id, "allow") {
		t.Fatal("resolve after drop must fail")
	}
	c.drop(id) // must be idempotent
}

func TestChoiceTimeoutDefaults(t *testing.T) {
	if got := choiceTimeout(0); got != defaultChoiceTimeout {
		t.Fatalf("want default for 0, got %v", got)
	}
	if got := choiceTimeout(-5); got != defaultChoiceTimeout {
		t.Fatalf("want default for negative, got %v", got)
	}
	if got := choiceTimeout(5000); got != 5*time.Second {
		t.Fatalf("want 5s, got %v", got)
	}
	if got := choiceTimeout(99999); got != maxChoiceTimeout {
		t.Fatalf("want clamp to %v (under the 30s enclosing call timeout), got %v", maxChoiceTimeout, got)
	}
}
