package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func validCompactionSummary() string {
	return strings.Join([]string{
		"## Primary Request and Intent",
		"- keep the original request",
		"## Key Technical Concepts",
		"- go",
		"## Files and Code",
		"- plugins/prompter/compact.go",
		"## Errors and Fixes",
		"- none",
		"## Pending Tasks",
		"- none",
		"## Current Work",
		"- compaction",
		"## Next Step",
		"- verify",
		"## Critical Context",
		"- none",
	}, "\n")
}

func testConversationMessages() []pluginsdk.ChatMessage {
	return []pluginsdk.ChatMessage{
		{Seq: 1, Role: pluginsdk.RoleSystem, Text: "system"},
		{Seq: 2, Role: pluginsdk.RoleUser, Text: "runtime"},
		{Seq: 3, Role: pluginsdk.RoleUser, Text: "first request"},
		{Seq: 4, Role: pluginsdk.RoleAssistant, Text: "first answer"},
		{Seq: 5, Role: pluginsdk.RoleUser, Text: "second request"},
		{Seq: 6, Role: pluginsdk.RoleAssistant, ToolCalls: []pluginsdk.ToolCall{{
			ID:   "call-1",
			Type: "function",
			FunctionCall: pluginsdk.FunctionCall{
				Name:      "file_read",
				Arguments: json.RawMessage(`{"path":"a.go"}`),
			},
		}}},
		{Seq: 7, Role: pluginsdk.RoleTool, ToolCallID: "call-1", Text: "package main"},
		{Seq: 8, Role: pluginsdk.RoleAssistant, Text: "second answer"},
		{Seq: 9, Role: pluginsdk.RoleUser, Text: "third request"},
		{Seq: 10, Role: pluginsdk.RoleAssistant, Text: "third answer"},
	}
}

func TestEstimateTextTokens(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcde", 2},
		{"你好", 2},
		{"hello 世界", 4},
	}
	for _, item := range cases {
		if got := estimateTextTokens(item.text); got != item.want {
			t.Errorf("estimateTextTokens(%q) = %d, want %d", item.text, got, item.want)
		}
	}
}

func TestCompactBudgetDerivation(t *testing.T) {
	cfg := config{MaxContextTokens: 60000, CompactThreshold: 0.8}
	if window := compactWindow(cfg, 128000); window != 60000 {
		t.Fatalf("window = %d, want 60000", window)
	}
	if threshold := compactThresholdTokens(cfg, 60000); threshold != 48000 {
		t.Fatalf("threshold = %d, want 48000", threshold)
	}
	if retain := compactRetainTokens(cfg, 60000); retain != 9600 {
		t.Fatalf("retain = %d, want 9600", retain)
	}

	cfg = config{CompactThreshold: 0.5}
	if window := compactWindow(cfg, 100000); window != 100000 {
		t.Fatalf("window = %d, want 100000", window)
	}
	if threshold := compactThresholdTokens(cfg, 100000); threshold != 50000 {
		t.Fatalf("threshold = %d, want 50000", threshold)
	}
	if compactWindow(config{}, 0) != 0 {
		t.Fatal("unknown window should stay unknown")
	}
}

func TestPlanCompactionRetainsNewestTurn(t *testing.T) {
	messages := testConversationMessages()
	if len(messages) != 10 {
		t.Fatalf("fixture changed: %d messages", len(messages))
	}
	plan, ok := planCompaction(messages, 2, 0, 1)
	if !ok {
		t.Fatal("expected a compactable range")
	}
	if plan.cut != 8 {
		t.Fatalf("cut = %d, want 8", plan.cut)
	}
	if plan.coveredFromSeq != 3 || plan.coveredToSeq != 8 {
		t.Fatalf("covered seqs = %d..%d, want 3..8", plan.coveredFromSeq, plan.coveredToSeq)
	}
	if plan.messageCount != 6 {
		t.Fatalf("covered messages = %d, want 6", plan.messageCount)
	}
	if messages[plan.cut].Role != pluginsdk.RoleUser || messages[plan.cut].Text != "third request" {
		t.Fatalf("retained tail starts at %+v", messages[plan.cut])
	}
}

func TestPlanCompactionKeepsToolPairInRetainedTail(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Seq: 1, Role: pluginsdk.RoleSystem, Text: "system"},
		{Seq: 2, Role: pluginsdk.RoleUser, Text: "runtime"},
		{Seq: 3, Role: pluginsdk.RoleUser, Text: "first request"},
		{Seq: 4, Role: pluginsdk.RoleAssistant, Text: "first answer"},
		{Seq: 5, Role: pluginsdk.RoleUser, Text: "second request"},
		{Seq: 6, Role: pluginsdk.RoleAssistant, ToolCalls: []pluginsdk.ToolCall{{
			ID:   "call-1",
			Type: "function",
			FunctionCall: pluginsdk.FunctionCall{
				Name:      "file_read",
				Arguments: json.RawMessage(`{"path":"a.go"}`),
			},
		}}},
		{Seq: 7, Role: pluginsdk.RoleTool, ToolCallID: "call-1", Text: "package main"},
		{Seq: 8, Role: pluginsdk.RoleAssistant, Text: "second answer"},
	}
	plan, ok := planCompaction(messages, 2, 0, 1)
	if !ok {
		t.Fatal("expected a compactable range")
	}
	if plan.cut != 4 {
		t.Fatalf("cut = %d, want 4", plan.cut)
	}
	if plan.coveredToSeq != 4 {
		t.Fatalf("covered to seq %d, want 4", plan.coveredToSeq)
	}
	if messages[plan.cut].Seq != 5 {
		t.Fatalf("retained tail starts at seq %d, want 5", messages[plan.cut].Seq)
	}
}

func TestPlanCompactionRequiresRealUserMessage(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Seq: 1, Role: pluginsdk.RoleSystem, Text: "system"},
		{Seq: 2, Role: pluginsdk.RoleUser, Text: "runtime"},
		{Seq: 100, Role: pluginsdk.RoleUser, Text: "prior checkpoint"},
		{Seq: 101, Role: pluginsdk.RoleAssistant, Text: "work"},
		{Seq: 102, Role: pluginsdk.RoleUser, Text: "current request"},
		{Seq: 103, Role: pluginsdk.RoleAssistant, Text: "current answer"},
	}
	if _, ok := planCompaction(messages, 2, 100, 1); ok {
		t.Fatal("a checkpoint-only covered range must not compact again")
	}
}

func TestPlanCompactionAfterCheckpointMergesPriorSummary(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Seq: 1, Role: pluginsdk.RoleSystem, Text: "system"},
		{Seq: 2, Role: pluginsdk.RoleUser, Text: "runtime"},
		{Seq: 100, Role: pluginsdk.RoleUser, Text: "old checkpoint"},
		{Seq: 9, Role: pluginsdk.RoleUser, Text: "third request"},
		{Seq: 10, Role: pluginsdk.RoleAssistant, Text: "third answer"},
		{Seq: 11, Role: pluginsdk.RoleUser, Text: "fourth request"},
		{Seq: 12, Role: pluginsdk.RoleAssistant, Text: "fourth answer"},
	}
	plan, ok := planCompaction(messages, 2, 100, 1)
	if !ok {
		t.Fatal("expected a compactable range")
	}
	if plan.cut != 5 {
		t.Fatalf("cut = %d, want 5", plan.cut)
	}
	if plan.coveredFromSeq != 9 || plan.coveredToSeq != 10 {
		t.Fatalf("covered seqs = %d..%d, want 9..10", plan.coveredFromSeq, plan.coveredToSeq)
	}
}

func TestPlanCompactionZeroRetainKeepsNewestTurn(t *testing.T) {
	messages := testConversationMessages()
	plan, ok := planCompaction(messages, 2, 0, 0)
	if !ok {
		t.Fatal("expected a compactable range")
	}
	if plan.cut != 8 || plan.coveredToSeq != 8 {
		t.Fatalf("plan = %+v, want cut 8 and covered to 8", plan)
	}
}

func TestPlanCompactionSkipsSingleTurn(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Seq: 1, Role: pluginsdk.RoleSystem, Text: "system"},
		{Seq: 2, Role: pluginsdk.RoleUser, Text: "only request"},
		{Seq: 3, Role: pluginsdk.RoleAssistant, Text: "only answer"},
	}
	if _, ok := planCompaction(messages, 1, 0, 1); ok {
		t.Fatal("a single turn cannot be compacted by user-message boundaries")
	}
}

func TestParseCompactionSummaryStripsAnalysis(t *testing.T) {
	text := "<analysis>private reasoning</analysis>\n<summary>" + validCompactionSummary() + "</summary>"
	summary, err := parseCompactionSummary(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if strings.Contains(summary, "<analysis>") || strings.Contains(summary, "private reasoning") {
		t.Fatalf("analysis leaked into summary: %q", summary)
	}
	if !strings.Contains(summary, "## Primary Request and Intent") {
		t.Fatalf("summary lost content: %q", summary)
	}
}

func TestParseCompactionSummaryRejectsTruncatedOutput(t *testing.T) {
	if _, err := parseCompactionSummary("## Primary Request and Intent\n- no tags"); !errors.Is(err, errSummaryTruncated) {
		t.Fatalf("error = %v, want errSummaryTruncated", err)
	}
	text := "<summary>unfinished"
	if _, err := parseCompactionSummary(text); !errors.Is(err, errSummaryTruncated) {
		t.Fatalf("unfinished block error = %v, want errSummaryTruncated", err)
	}
	var validation *summaryValidationError
	if _, err := parseCompactionSummary("<summary>   </summary>"); !errors.As(err, &validation) {
		t.Fatalf("empty summary error = %v, want summaryValidationError", err)
	}
}

func TestValidateCompactionSummary(t *testing.T) {
	summary := validCompactionSummary()
	if err := validateCompactionSummary(summary, 10000, 12000); err != nil {
		t.Fatalf("valid summary rejected: %v", err)
	}

	var validation *summaryValidationError
	missing := strings.Replace(summary, "## Current Work", "## Current State", 1)
	if err := validateCompactionSummary(missing, 10000, 12000); !errors.As(err, &validation) {
		t.Fatalf("missing section error = %v, want summaryValidationError", err)
	}

	tooLong := validCompactionSummary() + "\n" + strings.Repeat("x", 40000)
	if err := validateCompactionSummary(tooLong, 10000, 100000); !errors.As(err, &validation) {
		t.Fatalf("non-shrinking summary error = %v, want summaryValidationError", err)
	}
	if err := validateCompactionSummary(summary, 10000, 10); !errors.As(err, &validation) {
		t.Fatalf("over-cap summary error = %v, want summaryValidationError", err)
	}

	emptyIntent := strings.Replace(summary, "- keep the original request", "- (none)", 1)
	if err := validateCompactionSummary(emptyIntent, 10000, 12000); !errors.As(err, &validation) {
		t.Fatalf("empty intent error = %v, want summaryValidationError", err)
	}

	reordered := strings.Replace(summary, "## Key Technical Concepts", "## Temp Section", 1)
	reordered = strings.Replace(reordered, "## Critical Context", "## Key Technical Concepts", 1)
	if err := validateCompactionSummary(reordered, 10000, 12000); !errors.As(err, &validation) {
		t.Fatalf("out-of-order section error = %v, want summaryValidationError", err)
	}
}

func TestDeterministicCompactionSummary(t *testing.T) {
	messages := []pluginsdk.ChatMessage{
		{Role: pluginsdk.RoleUser, Text: strings.Repeat("a", compactMessageRunes+50)},
		{Role: pluginsdk.RoleAssistant, ToolCalls: []pluginsdk.ToolCall{{
			FunctionCall: pluginsdk.FunctionCall{Name: "file_read"},
		}}},
		{Role: pluginsdk.RoleTool, ToolCallID: "call-1", Text: "package main\n\nsecond paragraph"},
	}
	summary := deterministicCompactionSummary(messages, 600)
	if summary == "" {
		t.Fatal("deterministic summary is empty")
	}
	if !strings.Contains(summary, "- user: aaaa") {
		t.Fatalf("summary lost user line: %q", summary)
	}
	if !strings.Contains(summary, "[tool calls: file_read]") {
		t.Fatalf("summary lost tool call line: %q", summary)
	}
	if strings.Contains(summary, "second paragraph") {
		t.Fatalf("summary should keep only the first paragraph: %q", summary)
	}
	if utf8Len(summary) > 600 {
		t.Fatalf("summary exceeds max chars: %d", utf8Len(summary))
	}
}

func utf8Len(value string) int {
	return len([]rune(value))
}

func TestCheckpointTextWrapsSummary(t *testing.T) {
	text := checkpointText("summary body")
	if !strings.Contains(text, checkpointPreamble) {
		t.Fatalf("missing preamble: %q", text)
	}
	if !strings.Contains(text, "<compacted-summary>\nsummary body\n</compacted-summary>") {
		t.Fatalf("missing summary tags: %q", text)
	}
}

func TestProjectMessagesAppliesCompaction(t *testing.T) {
	meta := mustMarshal(t, compactionMeta{Source: "prompter", Method: compactMethodModel, CoveredToSeq: 4})
	facts := []sessionFact{
		{Seq: 1, Type: factTypeSystemPrompt, Role: pluginsdk.RoleSystem, TextContent: "system"},
		{Seq: 2, Type: factTypeRuntimeContext, Role: pluginsdk.RoleUser, TextContent: "runtime"},
		{Seq: 3, Type: factTypeMessage, Role: pluginsdk.RoleUser, TextContent: "first request"},
		{Seq: 4, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, TextContent: "first answer"},
		{Seq: 5, Type: factTypeMessage, Role: pluginsdk.RoleUser, TextContent: "second request"},
		{Seq: 6, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, TextContent: "second answer"},
		{Seq: 100, Type: factTypeContextCompaction, Role: "host", TextContent: "checkpoint", Meta: meta},
	}
	messages := projectMessages(facts)
	if len(messages) != 5 {
		t.Fatalf("messages = %d, want 5: %+v", len(messages), messages)
	}
	wantTexts := []string{"system", "runtime", "checkpoint", "second request", "second answer"}
	for i := range messages {
		if messages[i].Text != wantTexts[i] {
			t.Fatalf("message %d = %q, want %q", i, messages[i].Text, wantTexts[i])
		}
	}
	if messages[2].Seq != 100 || messages[2].Role != pluginsdk.RoleUser {
		t.Fatalf("checkpoint message = %+v", messages[2])
	}
	if contextPrefixCount(facts) != 2 {
		t.Fatalf("prefix count = %d, want 2", contextPrefixCount(facts))
	}
}

func TestProjectMessagesUsesLatestCompaction(t *testing.T) {
	older := mustMarshal(t, compactionMeta{CoveredToSeq: 3})
	newer := mustMarshal(t, compactionMeta{CoveredToSeq: 5})
	facts := []sessionFact{
		{Seq: 1, Type: factTypeSystemPrompt, Role: pluginsdk.RoleSystem, TextContent: "system"},
		{Seq: 2, Type: factTypeMessage, Role: pluginsdk.RoleUser, TextContent: "first"},
		{Seq: 3, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, TextContent: "first answer"},
		{Seq: 4, Type: factTypeMessage, Role: pluginsdk.RoleUser, TextContent: "second"},
		{Seq: 5, Type: factTypeMessage, Role: pluginsdk.RoleAssistant, TextContent: "second answer"},
		{Seq: 6, Type: factTypeMessage, Role: pluginsdk.RoleUser, TextContent: "third"},
		{Seq: 10, Type: factTypeContextCompaction, Role: "host", TextContent: "old checkpoint", Meta: older},
		{Seq: 20, Type: factTypeContextCompaction, Role: "host", TextContent: "new checkpoint", Meta: newer},
	}
	messages := projectMessages(facts)
	if len(messages) != 3 {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[1].Text != "new checkpoint" || messages[1].Seq != 20 {
		t.Fatalf("checkpoint = %+v", messages[1])
	}
	if messages[2].Text != "third" {
		t.Fatalf("retained tail = %+v", messages[2])
	}
	if compactionCoveredTo(facts) != 5 {
		t.Fatalf("covered to = %d, want 5", compactionCoveredTo(facts))
	}
}

func TestContextPressureTokensUsesRecordedUsage(t *testing.T) {
	messages := []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hello"}}
	tools := toolCatalog{}
	facts := []sessionFact{{Seq: 1, Type: factTypeUsage, Meta: json.RawMessage(`{"prompt_tokens":1000}`)}}
	if got := contextPressureTokens(facts, messages, tools); got != 1000 {
		t.Fatalf("pressure = %d, want recorded usage 1000", got)
	}
	facts = []sessionFact{{Seq: 1, Type: factTypeUsage, Meta: json.RawMessage(`{"prompt_tokens":1}`)}}
	if got := contextPressureTokens(facts, messages, tools); got <= 1 {
		t.Fatalf("pressure = %d, want heuristic estimate", got)
	}
}

func TestContextPressureIgnoresUsageBeforeCompaction(t *testing.T) {
	messages := []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: "hello"}}
	facts := []sessionFact{
		{Seq: 1, Type: factTypeUsage, Meta: json.RawMessage(`{"prompt_tokens":1000}`)},
		{Seq: 2, Type: factTypeContextCompaction, TextContent: "checkpoint", Meta: mustMarshal(t, compactionMeta{CoveredToSeq: 3})},
	}
	if got := contextPressureTokens(facts, messages, toolCatalog{}); got >= 1000 {
		t.Fatalf("pressure = %d, stale usage should be ignored", got)
	}
}

func TestDeterministicSummaryKeepsPriorCheckpointBody(t *testing.T) {
	prior := checkpointText(strings.Repeat("b", compactMessageRunes+50))
	messages := []pluginsdk.ChatMessage{{Role: pluginsdk.RoleUser, Text: prior}}
	summary := deterministicCompactionSummary(messages, 2000)
	if strings.Contains(summary, "This session is being continued") {
		t.Fatalf("prior checkpoint preamble leaked: %q", summary)
	}
	if !strings.Contains(summary, "bbbb") {
		t.Fatalf("prior checkpoint body missing: %q", summary)
	}
}
