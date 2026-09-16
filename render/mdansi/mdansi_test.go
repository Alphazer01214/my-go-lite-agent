package mdansi

import (
	"strings"
	"testing"
)

// stripANSI removes ANSI escape sequences so tests can assert on visible text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestRenderBasicMarkdown(t *testing.T) {
	out := Render("# Title\n\nHello **bold** and `code`.\n\n- one\n- two\n")
	plain := stripANSI(out)
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
	plain := stripANSI(out)
	if !strings.Contains(plain, "fmt.Println(1)") {
		t.Fatalf("want code line: %q", plain)
	}
	if !strings.Contains(plain, "go") {
		t.Fatalf("want lang label: %q", plain)
	}
}

func TestRenderTableAndLink(t *testing.T) {
	out := Render("| a | b |\n|---|---|\n| 1 | 2 |\n\nSee [x](https://example.com).\n")
	plain := stripANSI(out)
	if !strings.Contains(plain, "│ a │ b │") {
		t.Fatalf("want table header row: %q", plain)
	}
	if !strings.Contains(plain, "│ 1 │ 2 │") {
		t.Fatalf("want table body row: %q", plain)
	}
	if !strings.Contains(plain, "https://example.com") {
		t.Fatalf("want link dest: %q", plain)
	}
}

func TestIndent(t *testing.T) {
	out := Indent("a\nb\n", "  ")
	if out != "  a\n  b\n" {
		t.Fatalf("got %q", out)
	}
}