package main

import "testing"

func TestNormalizePermissionMode(t *testing.T) {
	cases := map[string]string{
		"":                DefaultPermissionMode,
		"ask":             ModeAsk,
		"read_only":       ModeAsk, // legacy name (ADR-0034)
		"WORKSPACE_WRITE": ModeWorkspaceWrite,
		" full_access ":   ModeFullAccess,
		"nope":            DefaultPermissionMode,
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
	if got := EffectivePermissionMode(SessionMeta{PermissionMode: ModeAsk}); got != ModeAsk {
		t.Fatalf("explicit: %s", got)
	}
	if got := EffectivePermissionMode(SessionMeta{PermissionMode: ModeReadOnly}); got != ModeAsk {
		t.Fatalf("legacy read_only must map to ask: %s", got)
	}
}
