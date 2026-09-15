package main

import "testing"

func TestResolveCwdRequiresWorkspace(t *testing.T) {
	if _, err := resolveCwd("", ""); err == nil {
		t.Fatal("want no_workspace when workspace empty")
	}
}

func TestResolveCwdInsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	p, err := resolveCwd(ws, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("want path")
	}
	if _, err := resolveCwd(ws, "../out"); err == nil {
		t.Fatal("want path_escape")
	}
}
