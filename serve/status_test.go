package serve

import (
	"sync"
	"testing"
)

func TestStatusForSessionIsolatesTurns(t *testing.T) {
	s := &Server{}
	if s.isRunning() || s.StatusForSession("a") != "idle" {
		t.Fatal("want idle before run")
	}
	s.beginRunning("sess-a")
	if !s.isRunning() || !s.IsRunningOn("sess-a") {
		t.Fatalf("want running sess-a, list=%v", s.RunningSessions())
	}
	if s.StatusForSession("sess-a") != "running" {
		t.Fatal("owning session must be running")
	}
	if s.StatusForSession("sess-b") != "idle" {
		t.Fatal("other session must stay idle")
	}
	// Parallel: default Session can run while sess-a is running.
	s.beginRunning("")
	if !s.IsRunningOn("") || !s.IsRunningOn("sess-a") {
		t.Fatal("both default and sess-a must be running")
	}
	if len(s.RunningSessions()) != 2 {
		t.Fatalf("want 2 running, got %v", s.RunningSessions())
	}
	s.endRunning("sess-a")
	s.endRunning("")
	if s.isRunning() || s.StatusForSession("sess-a") != "idle" {
		t.Fatal("want idle after clear")
	}
}

func TestAcquireTurnParallelAcrossSessions(t *testing.T) {
	s := &Server{}
	a := s.acquireTurn("a")
	if a == nil {
		t.Fatal("acquire a")
	}
	if s.acquireTurn("a") != nil {
		t.Fatal("same session must not double-acquire")
	}
	b := s.acquireTurn("b")
	if b == nil {
		t.Fatal("different session must acquire in parallel")
	}
	c := s.acquireTurn("")
	if c == nil {
		t.Fatal("default session must run in parallel with a/b")
	}
	a.mu.Unlock()
	if s.acquireTurn("a") == nil {
		t.Fatal("re-acquire a after release")
	}
	b.mu.Unlock()
	c.mu.Unlock()
}

func TestCancelTurnOnOnlyTargetSession(t *testing.T) {
	s := &Server{}
	a := s.acquireTurn("a")
	b := s.acquireTurn("b")
	if a == nil || b == nil {
		t.Fatal("acquire")
	}
	defer a.mu.Unlock()
	defer b.mu.Unlock()
	s.CancelTurnOn("a")
	if !s.TurnCancelledOn("a") {
		t.Fatal("a should be cancelled")
	}
	if s.TurnCancelledOn("b") {
		t.Fatal("b must not cancel")
	}
}

func TestAcquireTurnConcurrentSmoke(t *testing.T) {
	s := &Server{}
	var wg sync.WaitGroup
	started := make(chan string, 8)
	for _, sid := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			st := s.acquireTurn(id)
			if st == nil {
				return
			}
			s.beginRunning(id)
			started <- id
			s.endRunning(id)
			st.mu.Unlock()
		}(sid)
	}
	wg.Wait()
	close(started)
	n := 0
	for range started {
		n++
	}
	if n != 4 {
		t.Fatalf("want 4 parallel turns, got %d", n)
	}
}
