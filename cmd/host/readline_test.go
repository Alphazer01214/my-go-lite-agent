package main

import (
	"reflect"
	"testing"

	"github.com/tomori/my-go-lite-agent/plugin"
)

func TestCompleteSlashNativeAndPlugins(t *testing.T) {
	cp := &commandPlane{
		manifests: map[string]plugin.Manifest{
			"llm-openai": {
				Name: "llm-openai",
				Commands: []plugin.CommandSpec{
					{Name: "config"},
				},
			},
			"session": {Name: "session"},
		},
		mounted: []string{"llm-openai", "session"},
	}

	got := cp.completeSlash("/h")
	want := []string{"/help"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}

	got = cp.completeSlash("/")
	if len(got) < 4 {
		t.Fatalf("want native+plugins, got %v", got)
	}

	got = cp.completeSlash("/llm-openai ")
	want = []string{"/llm-openai config"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subcommands got %v want %v", got, want)
	}

	got = cp.completeSlash("/llm-openai c")
	want = []string{"/llm-openai config"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subcommand prefix got %v want %v", got, want)
	}

	if got := cp.completeSlash("hello"); got != nil {
		t.Fatalf("non-slash should be nil: %v", got)
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	if got := longestCommonPrefix([]string{"/help", "/hello"}); got != "/hel" {
		t.Fatalf("got %q", got)
	}
	if got := longestCommonPrefix([]string{"/a", "/b"}); got != "/" {
		t.Fatalf("got %q", got)
	}
}
