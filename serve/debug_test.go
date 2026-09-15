package serve

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestSetDebug(t *testing.T) {
	defer SetDebug(false)
	SetDebug(true)
	if !DebugEnabled() {
		t.Fatal("expected debug enabled")
	}
	SetDebug(false)
	if DebugEnabled() {
		t.Fatal("expected debug disabled")
	}
}

func TestTruncateDebugPayload(t *testing.T) {
	empty, n := truncateDebugPayload(nil, 10)
	if empty != "-" || n != 0 {
		t.Fatalf("empty: got %q n=%d", empty, n)
	}
	short, n := truncateDebugPayload(json.RawMessage(`{"a":1}`), 100)
	if short != `{"a":1}` || n != 7 {
		t.Fatalf("short: got %q n=%d", short, n)
	}
	longRaw := json.RawMessage(strings.Repeat("x", 50))
	long, n := truncateDebugPayload(longRaw, 10)
	if n != 50 {
		t.Fatalf("long n=%d", n)
	}
	if !strings.HasPrefix(long, strings.Repeat("x", 10)) || !strings.Contains(long, "…(+40)") {
		t.Fatalf("long truncated form: %q", long)
	}
}

func TestDebugFrameLogsToStderr(t *testing.T) {
	defer SetDebug(false)
	SetDebug(true)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	debugFrame("->", "llm", &protocol.Frame{
		ID:      "host-1",
		Type:    protocol.TypeReq,
		Cap:     "llm",
		Method:  "complete",
		Payload: json.RawMessage(`{"hello":"world"}`),
	})
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	out := buf.String()
	for _, want := range []string{"[debug]", "->", "plugin=llm", "id=host-1", "type=req", "cap=llm", "method=complete", `{"hello":"world"}`} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q:\n%s", want, out)
		}
	}
}

func TestDebugFrameSilentWhenOff(t *testing.T) {
	SetDebug(false)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	debugFrame("->", "llm", &protocol.Frame{ID: "1", Type: protocol.TypeReq})
	debugf("should not appear")
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	if buf.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", buf.String())
	}
}
