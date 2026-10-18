package protocol

import (
	"bytes"
	"testing"
)

func TestCancelFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := &Frame{
		Version:    2,
		ID:         "caller-1",
		Type:       FrameCancel,
		ContextID:  "ctx-1",
		Capability: "llm",
		Method:     "complete",
		Reason:     "deadline_exceeded",
	}
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != FrameCancel || out.Reason != "deadline_exceeded" || out.ID != "caller-1" {
		t.Fatalf("frame = %#v", out)
	}
}
