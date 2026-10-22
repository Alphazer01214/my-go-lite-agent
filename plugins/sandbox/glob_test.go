package main

import (
	"path/filepath"
	"testing"
)

func TestMatchPath(t *testing.T) {
	// A temp dir is absolute on every platform. A bare "\ws" is only rooted on
	// Windows, and filepath.IsAbs rejects it, which would double the workspace.
	workspace := t.TempDir()
	outside := t.TempDir()
	cases := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"relative both sides", "README.md", "README.md", true},
		{"relative pattern absolute path", "README.md", filepath.Join(workspace, "README.md"), true},
		{"outside the workspace", "README.md", filepath.Join(outside, "README.md"), false},
		{"star stays in one segment", "*.md", filepath.Join(workspace, "README.md"), true},
		{"star does not cross a segment", "*", filepath.Join(workspace, "docs", "a.md"), false},
		{"double star crosses segments", "**/*.md", filepath.Join(workspace, "docs", "a.md"), true},
		{"double star at the root", filepath.Join(workspace, "**"), filepath.Join(workspace, "a", "b"), true},
		{"prefix mismatch", "src/**", filepath.Join(workspace, "docs", "a"), false},
		{"question mark matches one character", "a?.txt", filepath.Join(workspace, "ab.txt"), true},
		{"empty path never matches", "**", "", false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := matchPath(item.pattern, item.path, workspace)
			if got != item.want {
				t.Fatalf("matchPath(%q, %q) = %v, want %v", item.pattern, item.path, got, item.want)
			}
		})
	}
}

func TestMatchPathWithoutWorkspace(t *testing.T) {
	if !matchPath("docs/*.md", "docs/a.md", "") {
		t.Fatal("relative pattern and path should match without a workspace")
	}
	if matchPath("docs/*.md", "other/a.md", "") {
		t.Fatal("different directories should not match")
	}
}

func TestMatchCommand(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		command string
		want    bool
	}{
		{"exact", "go test", "go test", true},
		{"prefix with wildcard", "rm -rf *", "rm -rf build", true},
		{"different program", "rm -rf *", "git status", false},
		{"wildcard inside a token", "git *", "git commit -m message", true},
		{"pattern longer than the command", "rm -rf * extra", "rm -rf build", false},
		{"case sensitive", "RM -rf *", "rm -rf build", false},
		{"empty pattern", "", "rm -rf build", false},
		{"question mark", "go tes?", "go test", true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := matchCommand(item.pattern, item.command)
			if got != item.want {
				t.Fatalf("matchCommand(%q, %q) = %v, want %v", item.pattern, item.command, got, item.want)
			}
		})
	}
}
