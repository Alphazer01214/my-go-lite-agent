package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestMatchPath(t *testing.T) {
	workspace := t.TempDir()
	absolutePattern := filepath.ToSlash(filepath.Join(workspace, "**"))
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*.go", "src/a/b.go", true},
		{"src/**", "src/a/b.go", true},
		{"src/*.go", "src/a/b.go", false},
		{"src/*.go", "src/b.go", true},
		{"**/.env", ".env", true},
		{"**/.env", "config/.env", true},
		{"**/*.env", "config/.env", true},
		{"*.go", "src/b.go", false},
		{absolutePattern, "src/b.go", true},
	}
	for _, tc := range cases {
		if got := matchPath(tc.pattern, tc.path, workspace); got != tc.want {
			t.Errorf("matchPath(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestMatchPathNativeSeparators(t *testing.T) {
	workspace := t.TempDir()
	pattern := filepath.Join(workspace, "src", "**", "*.go")
	path := filepath.Join(workspace, "src", "a", "b.go")
	if !matchPath(pattern, path, workspace) {
		t.Fatalf("native separators should match: %q vs %q", pattern, path)
	}
}

func TestMatchPathWindowsBackslashes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path separators")
	}
	workspace := `E:\work\project`
	if !matchPath(`src\**\*.go`, `src\a\b.go`, workspace) {
		t.Fatal("backslash path should match after normalization")
	}
	if !matchPath(`E:\work\project\**`, `src\a\b.go`, workspace) {
		t.Fatal("absolute backslash pattern should match after normalization")
	}
}

func TestMatchCommand(t *testing.T) {
	cases := []struct {
		pattern string
		command string
		want    bool
	}{
		{"rm -rf *", "rm -rf build/x", true},
		{"rm -rf *", "rm -rf build more", true},
		{"git *", "git status", true},
		{"git *", "npm install", false},
		{"git status", "git status --short", true},
		{"git status", "git stash", false},
	}
	for _, tc := range cases {
		if got := matchCommand(tc.pattern, tc.command); got != tc.want {
			t.Errorf("matchCommand(%q, %q) = %v, want %v", tc.pattern, tc.command, got, tc.want)
		}
	}
}
