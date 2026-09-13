package serve

import (
	"encoding/json"
	"testing"
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

func TestJSONToPairsOrder(t *testing.T) {
	raw := json.RawMessage(`{"path":"a.go","lines":40,"flag":true}`)
	pairs, overflow := JSONToPairs(raw, 8)
	if len(pairs) != 3 {
		t.Fatalf("want 3 pairs, got %d: %+v", len(pairs), pairs)
	}
	if overflow != "" {
		t.Fatalf("unexpected overflow: %q", overflow)
	}
	if pairs[0].Key != "path" || pairs[1].Key != "lines" || pairs[2].Key != "flag" {
		t.Fatalf("order not preserved: %+v", pairs)
	}
}

func TestJSONToPairsMax(t *testing.T) {
	raw := json.RawMessage(`{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6,"g":7,"h":8,"i":9,"j":10}`)
	pairs, overflow := JSONToPairs(raw, 8)
	if len(pairs) != 8 {
		t.Fatalf("want 8 pairs, got %d", len(pairs))
	}
	if overflow == "" {
		t.Fatal("want overflow text for remaining keys")
	}
}

func TestJSONToPairsNonObject(t *testing.T) {
	pairs, _ := JSONToPairs(json.RawMessage(`"hello"`), 8)
	if len(pairs) != 1 || pairs[0].Key != "value" {
		t.Fatalf("got %+v", pairs)
	}
}

func TestFormatRunningLine(t *testing.T) {
	if got := formatRunningLine("read_file"); got != "Running read_file…" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderIntentJSONShape(t *testing.T) {
	raw, err := json.Marshal(RenderIntent{Kind: KindMarkdownText, Text: "# hi", Level: "dim", Title: "t"})
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
