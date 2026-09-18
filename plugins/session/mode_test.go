package main

import "testing"

func TestNormalizePermissionMode(t *testing.T) {
	cases := map[string]string{
		"":              DefaultPermissionMode,
		"read_only":     ModeReadOnly,
		"WORKSPACE_WRITE": ModeWorkspaceWrite,
		" full_access ": ModeFullAccess,
		"nope":          DefaultPermissionMode,
	}
	for in, want := range cases {
		if got := NormalizePermissionMode(in); got != want {
			t.Fatalf("Normalize(%q)=%q want %q", in, got, want)
		}
	}
}

func TestEffectivePermissionMode(t *testing.T) {
	if got := EffectivePermissionMode(SessionMeta{}); got != ModeWorkspaceWrite {
		t.Fatalf("empty meta default: %s", got)
	}
	if got := EffectivePermissionMode(SessionMeta{PermissionMode: ModeReadOnly}); got != ModeReadOnly {
		t.Fatalf("explicit: %s", got)
	}
}
