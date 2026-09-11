package mdansi

import (
	"strings"
	"testing"
)

func TestRenderBasicMarkdown(t *testing.T) {
	out := Render("# Title\n\nHello **bold** and `code`.\n\n- one\n- two\n")
	plain := Plain(out)
	if !strings.Contains(plain, "Title") {
		t.Fatalf("want title: %q", plain)
	}
	if !strings.Contains(plain, "bold") || !strings.Contains(plain, "code") {
		t.Fatalf("want bold/code: %q", plain)
	}
	if !strings.Contains(plain, "• one") {
		t.Fatalf("want list bullet: %q", plain)
	}
}

func TestRenderFence(t *testing.T) {
	out := Render("```go\nfmt.Println(1)\n```\n")
	plain := Plain(out)
	if !strings.Contains(plain, "fmt.Println(1)") {
		t.Fatalf("want code line: %q", plain)
	}
	if !strings.Contains(plain, "go") {
		t.Fatalf("want lang label: %q", plain)
	}
}

func TestIndent(t *testing.T) {
	out := Indent("a\nb\n", "  ")
	if out != "  a\n  b\n" {
		t.Fatalf("got %q", out)
	}
}
