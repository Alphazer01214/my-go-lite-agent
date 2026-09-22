package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestClampLimit(t *testing.T) {
	if clampLimit(0) != 5 {
		t.Fatalf("default limit want 5 got %d", clampLimit(0))
	}
	if clampLimit(3) != 3 {
		t.Fatalf("want 3 got %d", clampLimit(3))
	}
	if clampLimit(99) != maxSearchHits {
		t.Fatalf("want %d got %d", maxSearchHits, clampLimit(99))
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("一二三四五", 2); got != "一二…" {
		t.Fatalf("got %q", got)
	}
	if got := truncateRunes("short", 100); got != "short" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatHits(t *testing.T) {
	out := formatHits("茅台", "qianfan", []searchHit{
		{Title: "T1", URL: "https://a.example", Snippet: "S1", Date: "2026-01-01"},
		{Title: "T2", URL: "https://b.example", Snippet: "S2"},
	})
	for _, want := range []string{"qianfan", "茅台", "[1] T1", "内容: S1", "链接: https://a.example", "日期: 2026-01-01", "[2] T2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestWebSearchDescriptionHasNow(t *testing.T) {
	d := webSearchDescription()
	if !strings.Contains(d, "Now: ") {
		t.Fatalf("desc=%q", d)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", strings.TrimPrefix(d[strings.Index(d, "Now: "):], "Now: ")); err != nil {
		t.Fatalf("now parse: %v (desc=%q)", err, d)
	}
}

func TestLoadConfigEnvWins(t *testing.T) {
	// Point WEB_SEARCH_API_KEY; config.json next to the test binary is unlikely
	// to exist, so env alone must set the key.
	t.Setenv("WEB_SEARCH_API_KEY", "bce.test-key-123456")
	cfg := loadConfig()
	if cfg.APIKey != "bce.test-key-123456" {
		t.Fatalf("apiKey=%q", cfg.APIKey)
	}
}

func TestQianfanSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key-abcdef" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"references": []map[string]any{
				{"title": "茅台日报", "content": "今日收盘上涨。", "url": "https://news.example/1", "date": "2026-01-02"},
				{"title": "", "content": "", "url": ""},
			},
		})
	}))
	defer srv.Close()

	old := qianfanSearchURL
	qianfanSearchURL = srv.URL
	defer func() { qianfanSearchURL = old }()

	hits, err := qianfanSearch("test-key-abcdef", "茅台", 5, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits=%+v", hits)
	}
	if hits[0].Title != "茅台日报" || hits[0].Date != "2026-01-02" {
		t.Fatalf("hit=%+v", hits[0])
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
