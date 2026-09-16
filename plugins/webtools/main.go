// Command webtools provides web_fetch and web_search (Phase 2 web tools).
//
// Capability: tools
//   - list → web_fetch, web_search (both readOnly)
//   - call → fetch a URL as text, or search via DuckDuckGo HTML
//
// No API key required. Network is best-effort; failures return FrameError.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const (
	defaultTimeout = 20 * time.Second
	maxBodyBytes   = 512 * 1024
	maxTextOut     = 24 * 1024
	maxSearchHits  = 8
	userAgent      = "liteagent-webtools/0.1 (+https://local)"
)

var toolSchemas = []map[string]any{
	{
		"name":        "web_fetch",
		"description": "Fetch a web URL and return readable text (HTML stripped). Only http/https.",
		"readOnly":    true,
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":       map[string]string{"type": "string", "description": "http(s) URL to fetch"},
				"timeoutMs": map[string]any{"type": "integer", "description": "Timeout ms (default 20000)"},
			},
			"required": []string{"url"},
		},
	},
	{
		"name":        "web_search",
		"description": "Search the web via DuckDuckGo HTML and return title/url/snippet hits.",
		"readOnly":    true,
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":     map[string]string{"type": "string", "description": "Search query"},
				"limit":     map[string]any{"type": "integer", "description": "Max hits (default 5, max 8)"},
				"timeoutMs": map[string]any{"type": "integer", "description": "Timeout ms (default 20000)"},
			},
			"required": []string{"query"},
		},
	},
}

func timeoutFrom(ms int) time.Duration {
	if ms <= 0 {
		return defaultTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func httpClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// validateURL enforces http(s) and rejects obvious private targets (lite SSRF guard).
func validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, &protocol.FrameError{Code: "bad_arguments", Message: "invalid url: " + err.Error()}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, &protocol.FrameError{Code: "bad_arguments", Message: "only http/https URLs are allowed"}
	}
	if u.Hostname() == "" {
		return nil, &protocol.FrameError{Code: "bad_arguments", Message: "url host required"}
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return nil, &protocol.FrameError{Code: "blocked_url", Message: "localhost URLs are not allowed"}
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return nil, &protocol.FrameError{Code: "blocked_url", Message: "private/link-local addresses are not allowed"}
	}
	return u, nil
}

func doGET(rawURL string, timeout time.Duration) (string, string, error) {
	u, err := validateURL(rawURL)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", "", &protocol.FrameError{Code: "fetch_failed", Message: err.Error()}
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	res, err := httpClient(timeout).Do(req)
	if err != nil {
		return "", "", &protocol.FrameError{Code: "fetch_failed", Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return "", "", &protocol.FrameError{Code: "http_error", Message: fmt.Sprintf("HTTP %d", res.StatusCode)}
	}
	limited := io.LimitReader(res.Body, maxBodyBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", "", &protocol.FrameError{Code: "fetch_failed", Message: err.Error()}
	}
	return string(body), res.Header.Get("Content-Type"), nil
}

var (
	reScript  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	reStyle   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	reNoscript = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`)
	reComment = regexp.MustCompile(`(?is)<!--.*?-->`)
	reTags    = regexp.MustCompile(`(?s)<[^>]+>`)
	reBlank   = regexp.MustCompile(`[ \t\r\f\v]+`)
	reLines   = regexp.MustCompile(`\n{3,}`)
)

func htmlToText(body string) string {
	s := body
	s = reScript.ReplaceAllString(s, " ")
	s = reStyle.ReplaceAllString(s, " ")
	s = reNoscript.ReplaceAllString(s, " ")
	s = reComment.ReplaceAllString(s, " ")
	// Prefer paragraph-ish breaks before stripping tags.
	s = regexp.MustCompile(`(?i)</(p|div|br|li|h[1-6]|tr)>`).ReplaceAllString(s, "\n")
	s = reTags.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = reBlank.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	var out []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	s = strings.Join(out, "\n")
	s = reLines.ReplaceAllString(s, "\n\n")
	if len(s) > maxTextOut {
		s = s[:maxTextOut] + "\n... (truncated)\n"
	}
	return strings.TrimSpace(s)
}

func handleFetch(args json.RawMessage) (string, error) {
	var in struct {
		URL       string `json:"url"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if strings.TrimSpace(in.URL) == "" {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: "url is required"}
	}
	body, ctype, err := doGET(in.URL, timeoutFrom(in.TimeoutMs))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "url=%s\ncontent-type=%s\n\n", in.URL, ctype)
	if strings.Contains(strings.ToLower(ctype), "html") || strings.Contains(body, "<html") {
		text := htmlToText(body)
		if text == "" {
			text = "(no extractable text)"
		}
		b.WriteString(text)
	} else {
		if len(body) > maxTextOut {
			body = body[:maxTextOut] + "\n... (truncated)\n"
		}
		b.WriteString(body)
	}
	return b.String(), nil
}

type searchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// ddgSearch scrapes html.duckduckgo.com result links (no API key).
func ddgSearch(query string, limit int, timeout time.Duration) ([]searchHit, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > maxSearchHits {
		limit = maxSearchHits
	}
	form := url.Values{}
	form.Set("q", query)
	endpoint := "https://html.duckduckgo.com/html/?" + form.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &protocol.FrameError{Code: "search_failed", Message: err.Error()}
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := httpClient(timeout).Do(req)
	if err != nil {
		return nil, &protocol.FrameError{Code: "search_failed", Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, &protocol.FrameError{Code: "http_error", Message: fmt.Sprintf("HTTP %d", res.StatusCode)}
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes))
	if err != nil {
		return nil, &protocol.FrameError{Code: "search_failed", Message: err.Error()}
	}
	hits := parseDDGHTML(string(raw), limit)
	if len(hits) == 0 {
		return nil, &protocol.FrameError{Code: "no_results", Message: "no search hits (page shape may have changed)"}
	}
	return hits, nil
}

var (
	reResultBlock = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	reSnippet     = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
	reDDGLink     = regexp.MustCompile(`(?:^|//)duckduckgo\.com/l/\?uddg=([^&]+)`)
)

func parseDDGHTML(html string, limit int) []searchHit {
	var hits []searchHit
	blocks := reResultBlock.FindAllStringSubmatch(html, -1)
	snippets := reSnippet.FindAllStringSubmatch(html, -1)
	for i, m := range blocks {
		if len(hits) >= limit {
			break
		}
		href := strings.TrimSpace(m[1])
		title := htmlToText(m[2])
		if strings.HasPrefix(href, "//") {
			href = "https:" + href
		}
		// DDG redirect → real URL
		if sm := reDDGLink.FindStringSubmatch(href); sm != nil {
			if dec, err := url.QueryUnescape(sm[1]); err == nil {
				href = dec
			}
		}
		snip := ""
		if i < len(snippets) && len(snippets[i]) > 1 {
			snip = htmlToText(snippets[i][1])
		}
		if title == "" && href == "" {
			continue
		}
		hits = append(hits, searchHit{Title: title, URL: href, Snippet: snip})
	}
	return hits
}

func handleSearch(args json.RawMessage) (string, error) {
	var in struct {
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
		TimeoutMs int    `json:"timeoutMs"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if strings.TrimSpace(in.Query) == "" {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: "query is required"}
	}
	hits, err := ddgSearch(in.Query, in.Limit, timeoutFrom(in.TimeoutMs))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "query=%q hits=%d\n", in.Query, len(hits))
	for i, h := range hits {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, h.Title, h.URL)
		if h.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", h.Snippet)
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func main() {
	s := pluginsdk.New()
	s.Handle("tools", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"tools": toolSchemas})
	})
	s.Handle("tools", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		var content string
		var err error
		switch in.Name {
		case "web_fetch":
			content, err = handleFetch(in.Arguments)
		case "web_search":
			content, err = handleSearch(in.Arguments)
		default:
			return nil, &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
		}
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"content": content})
	})
	_ = s.Serve()
}
