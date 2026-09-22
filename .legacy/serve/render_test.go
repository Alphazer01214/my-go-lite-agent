package serve

import (
	"encoding/json"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestTruncateRunes(t *testing.T) {
	if got := TruncateRunes("abc", 10); got != "abc" {
		t.Fatalf("short string changed: %q", got)
	}
	if got := TruncateRunes("hello world", 5); got != "hell…" {
		t.Fatalf("got %q", got)
	}
	// Chinese is rune-counted.
	if got := TruncateRunes("你好世界", 2); got != "你…" {
		t.Fatalf("cjk truncate: %q", got)
	}
}

func TestRenderIntentJSONShape(t *testing.T) {
	raw, err := json.Marshal(RenderIntent{Kind: pluginsdk.RenderKind(KindMarkdownText), Text: "# hi", Level: "dim", Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["kind"] != KindMarkdownText {
		t.Fatalf("want lowercase kind, got %v", m["kind"])
	}
	if m["text"] != "# hi" {
		t.Fatalf("want lowercase text, got %v", m["text"])
	}
	// Default Session uses empty id — field must still be present for Web filtering.
	if _, ok := m["sessionId"]; !ok {
		t.Fatal("sessionId must be present even when empty (default Session)")
	}
	if m["sessionId"] != "" {
		t.Fatalf("want empty sessionId, got %v", m["sessionId"])
	}
}
