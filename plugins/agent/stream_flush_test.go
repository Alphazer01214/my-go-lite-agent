package main

import (
	"testing"
	"time"
)

func TestStreamFlushPolicyCoalescesFastDeltas(t *testing.T) {
	policy := newStreamFlushPolicy()
	base := time.Unix(100, 0)
	flushes := 0
	for i := 0; i < 100; i++ {
		if policy.observe(base) {
			flushes++
		}
	}
	if flushes > 6 {
		t.Fatalf("fast deltas produced %d flushes", flushes)
	}
	if flushes < 2 {
		t.Fatalf("fast deltas produced %d flushes", flushes)
	}
}

func TestStreamFlushPolicyKeepsSlowDeltasVisible(t *testing.T) {
	policy := newStreamFlushPolicy()
	base := time.Unix(100, 0)
	flushes := 0
	for i := 0; i < 5; i++ {
		if policy.observe(base.Add(time.Duration(i) * 150 * time.Millisecond)) {
			flushes++
		}
	}
	if flushes != 5 {
		t.Fatalf("slow deltas produced %d flushes", flushes)
	}
}
