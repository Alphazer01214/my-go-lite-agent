package main

import (
	"errors"
	"strings"
	"testing"
)

// TestPolicyDecisionOnError pins the fail-closed decision table (BUG-01):
// a mounted-but-broken policy gate must deny, not silently allow; only the
// "no policy provider at all" case keeps the historical allow behavior.
func TestPolicyDecisionOnError(t *testing.T) {
	gateErr := errors.New("call to sandbox timed out after 30s")

	action, reason := policyDecisionOnError(gateErr, true)
	if action != "deny" {
		t.Fatalf("mounted-but-timed-out gate: want deny, got %q (%s)", action, reason)
	}
	if !strings.Contains(reason, "timed out") {
		t.Fatalf("want the root cause in the reason, got %q", reason)
	}

	action, reason = policyDecisionOnError(gateErr, false)
	if action != "allow" {
		t.Fatalf("no policy provider: want allow (historical), got %q", action)
	}
	if reason != "no policy provider" {
		t.Fatalf("want 'no policy provider' reason, got %q", reason)
	}
}