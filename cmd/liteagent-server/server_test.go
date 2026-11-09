package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestTraceToSummarySkipsChunkFrames(t *testing.T) {
	var out bytes.Buffer
	trace := traceTo(&out, "summary")
	trace("in", "llm", &protocol.Frame{ID: "1", Type: protocol.FrameEvent, Capability: "llm", Method: "chunk", Payload: []byte(`{"delta":"x"}`)})
	trace("in", "llm", &protocol.Frame{ID: "2", Type: protocol.FrameRequest, Capability: "llm", Method: "stream", Payload: []byte(`{}`)})
	logged := out.String()
	if strings.Contains(logged, "method=chunk") {
		t.Fatalf("summary trace kept chunk frame: %s", logged)
	}
	if !strings.Contains(logged, "method=stream") {
		t.Fatalf("summary trace dropped stream frame: %s", logged)
	}
}

func TestTraceToFullKeepsChunkFrames(t *testing.T) {
	var out bytes.Buffer
	trace := traceTo(&out, "full")
	trace("in", "llm", &protocol.Frame{ID: "1", Type: protocol.FrameEvent, Capability: "llm", Method: "chunk", Payload: []byte(`{"delta":"x"}`)})
	if !strings.Contains(out.String(), "method=chunk") {
		t.Fatalf("full trace dropped chunk frame: %s", out.String())
	}
}

func TestTraceToOffDisablesLogging(t *testing.T) {
	var out bytes.Buffer
	trace := traceTo(&out, "off")
	if trace != nil {
		t.Fatal("off trace must be nil")
	}
}
