package plugin

import "testing"

func TestManifestAutostartDependsOn(t *testing.T) {
	m := Manifest{
		Name: "agent", Version: "0.1.0", Protocol: 3,
		Entry: "agent", Autostart: true,
		DependsOn: []string{"session", "llm-openai"},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if !m.Autostart || len(m.DependsOn) != 2 {
		t.Fatalf("parsed fields: %+v", m)
	}
}

func TestManifestDependsOnRejectsSelfAndBadName(t *testing.T) {
	m := Manifest{Name: "agent", Version: "1", Protocol: 3, Entry: "a", DependsOn: []string{"agent"}}
	if err := m.Validate(); err == nil {
		t.Fatal("want self-dependsOn error")
	}
	m2 := Manifest{Name: "agent", Version: "1", Protocol: 3, Entry: "a", DependsOn: []string{"Session"}}
	if err := m2.Validate(); err == nil {
		t.Fatal("want bad name error")
	}
}
