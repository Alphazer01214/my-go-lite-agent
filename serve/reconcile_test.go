package serve

import (
	"testing"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
)

// testProc builds a fake Plugin process record (no executable — reconcile only
// reads manifest metadata and health).
func testProc(name string, provides, consumes []string, healthy bool) *proc {
	return &proc{
		found: discovery.Found{
			Manifest: plugin.Manifest{Name: name, Provides: provides, Consumes: consumes},
		},
		healthy: healthy,
	}
}

// TestReconcileConsumesClearsDegradedAndRegressesProvides is the ADR-0022
// recovery scenario: a consumer whose dependency is missing degrades and its
// provides withdraw; once the provider mounts, reconcile restores both.
func TestReconcileConsumesClearsDegradedAndRegressesProvides(t *testing.T) {
	s := &Server{
		plugins: map[string]*proc{
			"app": testProc("app", []string{"foo"}, []string{"llm"}, true),
		},
		provides: map[string]string{"foo": "app"},
		degraded: map[string]bool{},
	}
	s.reconcileConsumes()
	if _, ok := s.provides["foo"]; ok {
		t.Fatal("foo must be withdrawn while app is degraded")
	}
	if !s.degraded["app"] {
		t.Fatal("app must be degraded while llm is missing")
	}

	// Provider mounts: degraded clears, provides regress in the same pass.
	s.plugins["llm-openai"] = testProc("llm-openai", []string{"llm"}, nil, true)
	s.reconcileConsumes()
	if s.degraded["app"] {
		t.Fatal("app must recover once llm is provided")
	}
	if got := s.provides["foo"]; got != "app" {
		t.Fatalf("foo must return to app, got %q", got)
	}
	if got := s.provides["llm"]; got != "llm-openai" {
		t.Fatalf("llm owner must be llm-openai, got %q", got)
	}
}

// TestReconcileConsumesIsIdempotentAndIncrementsGen checks that running the
// same reconcile twice yields the same registry (idempotent) and bumps the
// generation counter each time (observability).
func TestReconcileConsumesIsIdempotentAndIncrementsGen(t *testing.T) {
	s := &Server{
		plugins: map[string]*proc{
			"app":       testProc("app", []string{"foo"}, []string{"llm"}, true),
			"llm-openai": testProc("llm-openai", []string{"llm"}, nil, true),
		},
		provides: map[string]string{"foo": "app", "llm": "llm-openai"},
		degraded: map[string]bool{},
	}
	s.reconcileConsumes()
	gen1, snap1 := s.reconcileGen, s.Registry()
	s.reconcileConsumes()
	gen2, snap2 := s.reconcileGen, s.Registry()
	if gen2 != gen1+1 {
		t.Fatalf("reconcileGen must increment, got %d -> %d", gen1, gen2)
	}
	if snap1.Provides["foo"] != snap2.Provides["foo"] || snap1.Provides["llm"] != snap2.Provides["llm"] {
		t.Fatalf("idempotent registry deviated: %+v -> %+v", snap1.Provides, snap2.Provides)
	}
	if len(snap2.Degraded) != 0 {
		t.Fatalf("no degraded expected, got %v", snap2.Degraded)
	}
	if len(snap2.Faces) != 0 {
		t.Fatalf("no faces expected, got %v", snap2.Faces)
	}
}

// TestReconcileConsumesUnhealthyPluginWithdraws checks that an unhealthy
// process cannot serve capabilities (crash path) and recovers on restart.
func TestReconcileConsumesUnhealthyPluginWithdraws(t *testing.T) {
	s := &Server{
		plugins: map[string]*proc{
			"llm-openai": testProc("llm-openai", []string{"llm"}, nil, false),
		},
		provides: map[string]string{"llm": "llm-openai"},
		degraded: map[string]bool{},
	}
	s.reconcileConsumes()
	if _, ok := s.provides["llm"]; ok {
		t.Fatal("unhealthy plugin must not provide llm")
	}
	s.plugins["llm-openai"] = testProc("llm-openai", []string{"llm"}, nil, true)
	s.reconcileConsumes()
	if got := s.provides["llm"]; got != "llm-openai" {
		t.Fatalf("healthy plugin must provide llm again, got %q", got)
	}
}