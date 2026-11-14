package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	defaultCompactThreshold    = 0.8
	defaultCompactMaxChars     = 12000
	defaultRetainRatio         = 0.16
	approxCharsPerToken        = 4
	compactMessageRunes        = 400
	compactPurpose             = "compaction"
	compactMethodModel         = "model"
	compactMethodDeterministic = "deterministic"
)

const checkpointPreamble = "This session is being continued from a previous conversation that ran out of context. " +
	"The summary below covers the earlier portion.\n" +
	"Resume directly — do not acknowledge the summary, do not recap, do not re-plan what was already decided."

const compactionInstruction = `CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.
- Do NOT use file_read, shell_execute, file_grep, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM
that will resume the task. Output EXACTLY the Markdown structure below: keep every section, in
order. Use terse bullets, not prose paragraphs. Write "(none)" for an empty section — never drop
a section.

## Primary Request and Intent
- [the user's original and evolving goals, block-quoted verbatim where the exact wording matters]
## Key Technical Concepts
- [technologies, frameworks, patterns, and conventions in play]
## Files and Code
- [exact workspace-relative path: why it matters, key changes or snippets]
## Errors and Fixes
- [error: how it was resolved, plus related user feedback]
## Pending Tasks
- [explicitly requested work not yet completed]
## Current Work
- [precisely what was in progress at this checkpoint]
## Next Step
- [the single next action, or "(none)"]
## Critical Context
- [decisions and their rationale, constraints, user preferences, open questions]

Rules:
- Preserve exact file paths, function names, and error messages.
- When you encounter an existing summary block, it is a PRIOR checkpoint: preserve still-true
  facts, drop stale ones, and merge newer information into one consolidated summary under the
  same structure. Do not copy it forward verbatim.
- Do NOT mention this summarization request or that the context was compacted.

REMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by
a <summary> block. Tool calls will be rejected and you will fail the task.`

var compactionSections = []string{
	"## Primary Request and Intent",
	"## Key Technical Concepts",
	"## Files and Code",
	"## Errors and Fixes",
	"## Pending Tasks",
	"## Current Work",
	"## Next Step",
	"## Critical Context",
}

type compactionMeta struct {
	Source         string `json:"source"`
	Method         string `json:"method"`
	Model          string `json:"model,omitempty"`
	CoveredFromSeq int    `json:"covered_from_seq,omitempty"`
	CoveredToSeq   int    `json:"covered_to_seq"`
	MessageCount   int    `json:"message_count"`
	TokensBefore   int    `json:"tokens_before"`
	TokensAfter    int    `json:"tokens_after,omitempty"`
	OverBudget     bool   `json:"over_budget,omitempty"`
	CreatedAt      string `json:"created_at"`
}

type compactResult struct {
	Compacted      bool   `json:"compacted"`
	Reason         string `json:"reason,omitempty"`
	Method         string `json:"method,omitempty"`
	CoveredFromSeq int    `json:"covered_from_seq,omitempty"`
	CoveredToSeq   int    `json:"covered_to_seq,omitempty"`
	TokensBefore   int    `json:"tokens_before,omitempty"`
	TokensAfter    int    `json:"tokens_after,omitempty"`
	OverBudget     bool   `json:"over_budget,omitempty"`
}

type compactionPlan struct {
	cut            int
	coveredFromSeq int
	coveredToSeq   int
	coveredTokens  int
	messageCount   int
}

type compactRequest struct {
	SessionID string
	Model     string
	ContextID string
	Facts     []sessionFact
	Messages  []pluginsdk.ChatMessage
	Tools     toolCatalog
	Manual    bool
}

type summaryValidationError struct {
	reason string
}

func (e *summaryValidationError) Error() string {
	return e.reason
}

var errSummaryTruncated = errors.New("compaction summary truncated")

func estimateTextTokens(text string) int {
	ascii, nonASCII := 0, 0
	for _, r := range text {
		if r < utf8.RuneSelf {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+approxCharsPerToken-1)/approxCharsPerToken + nonASCII
}

func estimateMessageTokens(message pluginsdk.ChatMessage) int {
	tokens := 4 + estimateTextTokens(message.Text)
	for _, call := range message.ToolCalls {
		tokens += 4 + estimateTextTokens(call.FunctionCall.Name) + estimateTextTokens(string(call.FunctionCall.Arguments))
	}
	return tokens
}

func estimateMessagesTokens(messages []pluginsdk.ChatMessage) int {
	total := 0
	for _, message := range messages {
		total += estimateMessageTokens(message)
	}
	return total
}

func estimateToolsTokens(tools []pluginsdk.Tool) int {
	total := 0
	for _, tool := range tools {
		total += 8 + estimateTextTokens(tool.Name) + estimateTextTokens(tool.Description) + estimateTextTokens(string(tool.Parameters))
	}
	return total
}

func compactWindow(cfg config, modelWindow int) int {
	window := modelWindow
	if cfg.MaxContextTokens > 0 && (window <= 0 || cfg.MaxContextTokens < window) {
		window = cfg.MaxContextTokens
	}
	return window
}

func compactThresholdTokens(cfg config, window int) int {
	ratio := cfg.CompactThreshold
	if ratio <= 0 {
		ratio = defaultCompactThreshold
	}
	return int(float64(window) * ratio)
}

func compactRetainTokens(cfg config, window int) int {
	if cfg.RetainRecentTokens > 0 {
		return cfg.RetainRecentTokens
	}
	return int(float64(window) * defaultRetainRatio)
}

func compactMaxChars(cfg config) int {
	if cfg.CompactMaxChars > 0 {
		return cfg.CompactMaxChars
	}
	return defaultCompactMaxChars
}

func contextPressureTokens(facts []sessionFact, messages []pluginsdk.ChatMessage, tools toolCatalog) int {
	estimated := estimateMessagesTokens(messages) + estimateToolsTokens(tools.Tools)
	usageFact := latestFact(facts, factTypeUsage)
	compactionFact := latestFact(facts, factTypeContextCompaction)
	// Usage recorded before the latest compaction describes a surface that no
	// longer exists; the projection estimate is the only truthful pressure.
	if usageFact == nil || (compactionFact != nil && usageFact.Seq < compactionFact.Seq) {
		return estimated
	}
	var usage usageMeta
	if len(usageFact.Meta) == 0 || json.Unmarshal(usageFact.Meta, &usage) != nil {
		return estimated
	}
	if usage.PromptTokens > estimated {
		return usage.PromptTokens
	}
	return estimated
}

// planCompaction keeps the newest user turn as the retained tail and never
// cuts inside a tool call/result pair: the cut always lands on a user message.
func planCompaction(messages []pluginsdk.ChatMessage, prefix, checkpointSeq, retainTokens int) (compactionPlan, bool) {
	if prefix < 0 {
		prefix = 0
	}
	if prefix >= len(messages) {
		return compactionPlan{}, false
	}
	conversation := messages[prefix:]
	if len(conversation) < 2 {
		return compactionPlan{}, false
	}

	cut := 0
	if retainTokens <= 0 {
		cut = len(conversation) - 1
		for cut > 0 && conversation[cut].Role != pluginsdk.RoleUser {
			cut--
		}
		if cut <= 0 {
			return compactionPlan{}, false
		}
	} else {
		start := len(conversation)
		tokens := 0
		for start > 0 && tokens < retainTokens {
			start--
			tokens += estimateMessageTokens(conversation[start])
		}
		if start == 0 {
			return compactionPlan{}, false
		}
		// The retain budget can land inside the newest turn. Move the cut back
		// to that turn's user message so the whole turn stays together.
		cut = start
		for cut > 0 && conversation[cut].Role != pluginsdk.RoleUser {
			cut--
		}
		if cut == 0 {
			return compactionPlan{}, false
		}
	}

	covered := conversation[:cut]
	realUserMessages := 0
	fromSeq, toSeq := 0, 0
	for _, message := range covered {
		if message.Role == pluginsdk.RoleUser && message.Seq != checkpointSeq {
			realUserMessages++
		}
		if message.Seq <= 0 || message.Seq == checkpointSeq {
			continue
		}
		if fromSeq == 0 {
			fromSeq = message.Seq
		}
		toSeq = message.Seq
	}
	if realUserMessages == 0 || toSeq == 0 {
		return compactionPlan{}, false
	}

	return compactionPlan{
		cut:            prefix + cut,
		coveredFromSeq: fromSeq,
		coveredToSeq:   toSeq,
		coveredTokens:  estimateMessagesTokens(covered),
		messageCount:   len(covered),
	}, true
}

func latestCompactionSeq(facts []sessionFact) int {
	if fact := latestFact(facts, factTypeContextCompaction); fact != nil {
		return fact.Seq
	}
	return 0
}

func parseCompactionSummary(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", &summaryValidationError{reason: "empty summary"}
	}
	lower := strings.ToLower(text)
	open := strings.Index(lower, "<summary>")
	if open < 0 {
		return "", errSummaryTruncated
	}
	closeIndex := strings.LastIndex(lower, "</summary>")
	if closeIndex < 0 || closeIndex <= open {
		return "", errSummaryTruncated
	}
	summary := strings.TrimSpace(text[open+len("<summary>") : closeIndex])
	if summary == "" {
		return "", &summaryValidationError{reason: "empty summary"}
	}
	return summary, nil
}

func validateCompactionSummary(summary string, coveredTokens, maxChars int) error {
	if maxChars > 0 && utf8.RuneCountInString(summary) > maxChars {
		return &summaryValidationError{reason: "summary exceeds compact_max_chars"}
	}
	cursor := 0
	for _, section := range compactionSections {
		next := strings.Index(summary[cursor:], section)
		if next < 0 {
			return &summaryValidationError{reason: "summary is missing section: " + section}
		}
		cursor += next + len(section)
	}
	intent := strings.TrimSpace(strings.TrimPrefix(compactionSectionBody(summary, "## Primary Request and Intent"), "-"))
	if intent == "" || strings.EqualFold(intent, "(none)") {
		return &summaryValidationError{reason: "summary has an empty intent section"}
	}
	if coveredTokens > 0 && estimateTextTokens(summary) >= coveredTokens {
		return &summaryValidationError{reason: "summary is not shorter than the covered content"}
	}
	return nil
}

func compactionSectionBody(summary, heading string) string {
	index := strings.Index(summary, heading)
	if index < 0 {
		return ""
	}
	body := summary[index+len(heading):]
	if next := strings.Index(body, "\n## "); next >= 0 {
		body = body[:next]
	}
	return strings.TrimSpace(body)
}

func (p *plugin) summarize(
	ctx context.Context,
	contextID, model string,
	replay []pluginsdk.ChatMessage,
	tools []pluginsdk.Tool,
	coveredTokens, maxChars int,
) (string, error) {
	input := make([]pluginsdk.ChatMessage, 0, len(replay)+1)
	input = append(input, replay...)
	input = append(input, pluginsdk.ChatMessage{Role: pluginsdk.RoleUser, Text: compactionInstruction})

	message, err := p.requestCompactionSummary(ctx, contextID, model, input, tools)
	if err != nil {
		return "", err
	}
	if len(message.ToolCalls) > 0 {
		return "", &summaryValidationError{reason: "summary response contains tool calls"}
	}
	summary, err := parseCompactionSummary(message.Text)
	if err != nil {
		return "", err
	}
	if err := validateCompactionSummary(summary, coveredTokens, maxChars); err != nil {
		return "", err
	}
	return summary, nil
}

func (p *plugin) requestCompactionSummary(
	ctx context.Context,
	contextID, model string,
	messages []pluginsdk.ChatMessage,
	tools []pluginsdk.Tool,
) (pluginsdk.ChatMessage, error) {
	if tools == nil {
		tools = []pluginsdk.Tool{}
	}
	payload, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"tools":    tools,
		"purpose":  compactPurpose,
	})
	if err != nil {
		return pluginsdk.ChatMessage{}, err
	}
	raw, err := p.sdk.CallInContext(ctx, contextID, "llm", "stream", payload)
	if err != nil {
		return pluginsdk.ChatMessage{}, err
	}
	var out struct {
		Messages []pluginsdk.ChatMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return pluginsdk.ChatMessage{}, err
	}
	if len(out.Messages) == 0 {
		return pluginsdk.ChatMessage{}, fmt.Errorf("compaction call returned no messages")
	}
	return out.Messages[0], nil
}

func deterministicCompactionSummary(messages []pluginsdk.ChatMessage, maxChars int) string {
	var builder strings.Builder
	used := 0
	for _, message := range messages {
		text := firstParagraph(message.Text)
		if text == "" && len(message.ToolCalls) > 0 {
			names := make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				names = append(names, call.FunctionCall.Name)
			}
			text = "[tool calls: " + strings.Join(names, ", ") + "]"
		}
		if text == "" {
			continue
		}
		line := "- " + message.Role + ": " + text
		width := utf8.RuneCountInString(line) + 1
		if used+width > maxChars {
			break
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
		used += width
	}
	return strings.TrimSpace(builder.String())
}

func firstParagraph(text string) string {
	text = strings.TrimSpace(text)
	if open := strings.Index(text, "<compacted-summary>"); open >= 0 {
		text = text[open+len("<compacted-summary>"):]
		if closeIndex := strings.Index(text, "</compacted-summary>"); closeIndex >= 0 {
			text = text[:closeIndex]
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if index := strings.Index(text, "\n\n"); index >= 0 {
		text = text[:index]
	}
	text = strings.Join(strings.Fields(text), " ")
	return truncateRunes(text, compactMessageRunes)
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "…"
}

func checkpointText(summary string) string {
	return checkpointPreamble + "\n\n<compacted-summary>\n" + strings.TrimSpace(summary) + "\n</compacted-summary>"
}

func (p *plugin) compactContext(
	ctx context.Context,
	in compactRequest,
) (compactResult, []sessionFact, []pluginsdk.ChatMessage, error) {
	noop := func(reason string) (compactResult, []sessionFact, []pluginsdk.ChatMessage, error) {
		return compactResult{Compacted: false, Reason: reason}, in.Facts, in.Messages, nil
	}

	cfg := p.currentConfig()
	if !in.Manual && !compactionEnabled(cfg) {
		return noop("compaction disabled")
	}
	window := compactWindow(cfg, p.modelContextWindow(ctx, in.Model))
	if window <= 0 {
		return noop("model context window is unknown")
	}

	tokensBefore := contextPressureTokens(in.Facts, in.Messages, in.Tools)
	threshold := compactThresholdTokens(cfg, window)
	if !in.Manual && tokensBefore < threshold {
		return noop("below compaction threshold")
	}

	prefix := contextPrefixCount(in.Facts)
	plan, ok := planCompaction(in.Messages, prefix, latestCompactionSeq(in.Facts), compactRetainTokens(cfg, window))
	if !ok {
		return noop("no compactable range")
	}

	summary, err := p.summarize(
		ctx, in.ContextID, in.Model,
		in.Messages[:plan.cut], in.Tools.Tools,
		plan.coveredTokens, compactMaxChars(cfg),
	)
	method := compactMethodModel
	if err != nil {
		var validation *summaryValidationError
		if !errors.As(err, &validation) {
			logf("compaction summary skipped: %v", err)
			return noop("summary call failed")
		}
		summary = deterministicCompactionSummary(in.Messages[prefix:plan.cut], compactMaxChars(cfg))
		method = compactMethodDeterministic
		if summary == "" || estimateTextTokens(summary) >= plan.coveredTokens {
			logf("deterministic compaction summary skipped: %v", validation)
			return noop("summary rejected")
		}
		logf("compaction fell back to deterministic summary: %v", validation)
	}

	wrapped := checkpointText(summary)
	tokensAfter := estimateMessagesTokens(in.Messages[:prefix]) +
		estimateMessageTokens(pluginsdk.ChatMessage{Role: pluginsdk.RoleUser, Text: wrapped}) +
		estimateMessagesTokens(in.Messages[plan.cut:]) +
		estimateToolsTokens(in.Tools.Tools)

	meta := compactionMeta{
		Source:         "prompter",
		Method:         method,
		Model:          in.Model,
		CoveredFromSeq: plan.coveredFromSeq,
		CoveredToSeq:   plan.coveredToSeq,
		MessageCount:   plan.messageCount,
		TokensBefore:   tokensBefore,
		TokensAfter:    tokensAfter,
		OverBudget:     tokensAfter >= threshold,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return compactResult{}, nil, nil, err
	}
	if _, err := p.appendFacts(ctx, in.SessionID, []factIn{{
		Type:        factTypeContextCompaction,
		Role:        "host",
		TextContent: wrapped,
		Meta:        encoded,
	}}); err != nil {
		return compactResult{}, nil, nil, prompterError("append compaction", err)
	}

	facts, err := p.sessionFacts(ctx, in.SessionID)
	if err != nil {
		return compactResult{}, nil, nil, prompterError("reload session facts", err)
	}
	messages := projectMessages(facts)
	return compactResult{
		Compacted:      true,
		Method:         method,
		CoveredFromSeq: plan.coveredFromSeq,
		CoveredToSeq:   plan.coveredToSeq,
		TokensBefore:   tokensBefore,
		TokensAfter:    tokensAfter,
		OverBudget:     tokensAfter >= threshold,
	}, facts, messages, nil
}
