package main

import (
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestReadSSEDispatchesReasoningAndContentDeltas(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"choices":[{"delta":{"reasoning_content":"先分析"}}]}`,
		``,
		`: keep-alive`,
		`data: {"choices":[{"delta":{"reasoning":"再判断"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"你"}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":1}}`,
		``,
		`data: {"choices":[{"delta":{"content":"好"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var got []string
	if err := readSSE(strings.NewReader(stream), func(channel, delta string) error {
		got = append(got, channel+":"+delta)
		return nil
	}); err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	want := []string{"thinking:先分析", "thinking:再判断", "content:你", "content:好"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
}

func TestReadSSEStopsOnErrorChunk(t *testing.T) {
	stream := `data: {"error":{"message":"bad key"}}` + "\n"
	err := readSSE(strings.NewReader(stream), func(string, string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("err = %v, want upstream error", err)
	}
}

func TestWireMessagesOmitsReasoningFromUpstreamContent(t *testing.T) {
	messages := wireMessages([]pluginsdk.ChatMessage{{
		Role:      pluginsdk.RoleAssistant,
		Text:      "答案",
		Reasoning: "内部推理",
	}})
	if len(messages) != 1 || messages[0].Content != "答案" {
		t.Fatalf("wire messages = %+v, want content 答案", messages)
	}
}
