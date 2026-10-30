package main

import (
	"strings"
	"testing"
)

func TestReadSSEAssemblesContentDeltas(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"你"}}]}`,
		``,
		`: keep-alive`,
		`data: {"choices":[{"delta":{"content":"好"}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":1}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var got strings.Builder
	if err := readSSE(strings.NewReader(stream), func(delta string) error {
		got.WriteString(delta)
		return nil
	}); err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if got.String() != "你好" {
		t.Fatalf("text = %q, want 你好", got.String())
	}
}

func TestReadSSEStopsOnErrorChunk(t *testing.T) {
	stream := `data: {"error":{"message":"bad key"}}` + "\n"
	err := readSSE(strings.NewReader(stream), func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("err = %v, want upstream error", err)
	}
}
