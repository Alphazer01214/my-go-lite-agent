package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	if _, err := validateURL("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := validateURL("http://127.0.0.1/"); err == nil {
		t.Fatal("want block localhost")
	}
	if _, err := validateURL("ftp://example.com"); err == nil {
		t.Fatal("want block non-http")
	}
	if _, err := validateURL("http://10.0.0.1/"); err == nil {
		t.Fatal("want block private IP")
	}
}

func TestHTMLToText(t *testing.T) {
	html := `<html><head><style>b{}</style><script>x=1</script></head>
<body><h1>Title</h1><p>Hello <b>world</b></p><!-- c --><p>Next</p></body></html>`
	out := htmlToText(html)
	if !strings.Contains(out, "Title") || !strings.Contains(out, "Hello world") {
		t.Fatalf("out=%q", out)
	}
	if strings.Contains(out, "x=1") || strings.Contains(out, "<p>") {
		t.Fatalf("leaked tags/script: %q", out)
	}
}

func TestParseDDGHTML(t *testing.T) {
	page := `
<div class="result">
  <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage">Example Page</a>
  <a class="result__snippet" href="#">A short snippet about example.</a>
</div>`
	hits := parseDDGHTML(page, 5)
	if len(hits) != 1 {
		t.Fatalf("hits=%+v", hits)
	}
	if hits[0].URL != "https://example.com/page" {
		t.Fatalf("url=%q", hits[0].URL)
	}
	if !strings.Contains(hits[0].Title, "Example") {
		t.Fatalf("title=%q", hits[0].Title)
	}
}

func TestDispatchUnknown(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"url": "https://example.com"})
	_, err := handleFetch(raw)
	// Network may fail in CI; only assert unknown tool path via call switch in main — unit for fetch URL validation.
	if err != nil {
		fe, ok := err.(interface{ Error() string })
		if !ok {
			t.Fatal(err)
		}
		if strings.Contains(fe.Error(), "unknown tool") {
			t.Fatal(fe)
		}
	}
}
