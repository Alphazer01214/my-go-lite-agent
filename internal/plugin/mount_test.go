package plugin

import (
	"strings"
	"testing"
)

func disc(name string, autostart bool, depends ...string) Discovery {
	return Discovery{
		Dir: name,
		Manifest: Manifest{
			Name:      name,
			Autostart: autostart,
			DependsOn: depends,
		},
	}
}

func names(ordered []Discovery) []string {
	out := make([]string, 0, len(ordered))
	for _, d := range ordered {
		out = append(out, d.Manifest.Name)
	}
	return out
}

func TestAutostartClosureDepsBeforeDependents(t *testing.T) {
	// agent depends on session + llm-openai; all three autostart
	ordered, warns := AutostartClosure(Discoveries{Plugins: []Discovery{
		disc("agent", true, "session", "llm-openai"),
		disc("llm-openai", true),
		disc("session", true),
	}})
	if len(warns) != 0 {
		t.Fatalf("unexpected warns: %v", warns)
	}
	got := names(ordered)
	want := []string{"session", "llm-openai", "agent"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ordered = %v, want %v", got, want)
	}
}

func TestAutostartClosurePullsNonAutostartDependency(t *testing.T) {
	// dep is not autostart but pulled in by agent's depends_on
	ordered, warns := AutostartClosure(Discoveries{Plugins: []Discovery{
		disc("agent", true, "dep"),
		disc("dep", false),
	}})
	if len(warns) != 0 {
		t.Fatalf("unexpected warns: %v", warns)
	}
	got := names(ordered)
	if strings.Join(got, ",") != "dep,agent" {
		t.Fatalf("ordered = %v, want [dep agent]", got)
	}
}

func TestAutostartClosureSkipsNonAutostart(t *testing.T) {
	ordered, warns := AutostartClosure(Discoveries{Plugins: []Discovery{
		disc("agent", false),
		disc("session", true),
	}})
	if len(warns) != 0 {
		t.Fatalf("unexpected warns: %v", warns)
	}
	if got := names(ordered); strings.Join(got, ",") != "session" {
		t.Fatalf("ordered = %v, want [session]", got)
	}
}

func TestAutostartClosureMissingDependencySoftFails(t *testing.T) {
	ordered, warns := AutostartClosure(Discoveries{Plugins: []Discovery{
		disc("agent", true, "ghost"),
	}})
	if got := names(ordered); strings.Join(got, ",") != "agent" {
		t.Fatalf("ordered = %v, want [agent]", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "ghost") {
		t.Fatalf("warns = %v, want one mentioning ghost", warns)
	}
}

func TestAutostartClosureCycleSoftFails(t *testing.T) {
	ordered, warns := AutostartClosure(Discoveries{Plugins: []Discovery{
		disc("a", true, "b"),
		disc("b", true, "a"),
	}})
	// both still mount, exactly once each
	got := names(ordered)
	if len(got) != 2 {
		t.Fatalf("ordered = %v, want both a and b once", got)
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, "cycle") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warns = %v, want a cycle warning", warns)
	}
}

func TestAutostartClosureDuplicateNameSoftFails(t *testing.T) {
	ordered, warns := AutostartClosure(Discoveries{Plugins: []Discovery{
		disc("session", true),
		{Dir: "other", Manifest: Manifest{Name: "session", Autostart: true}},
	}})
	if got := names(ordered); len(got) != 1 || got[0] != "session" {
		t.Fatalf("ordered = %v, want single session", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "duplicate") {
		t.Fatalf("warns = %v, want one duplicate warning", warns)
	}
}
