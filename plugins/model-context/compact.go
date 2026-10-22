package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

func buildSummary(facts []fact, maxChars int) string {
	var b strings.Builder
	b.WriteString("Earlier conversation summary:\n")
	for _, item := range facts {
		content := strings.Join(strings.Fields(item.Content), " ")
		if len(content) > 400 {
			content = truncateUTF8(content, 400) + "..."
		}
		if content == "" && len(item.ToolCalls) == 0 {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item.Role)
		b.WriteString(": ")
		b.WriteString(content)
		if len(item.ToolCalls) > 0 {
			b.WriteString(" [tool_calls]")
		}
		b.WriteByte('\n')
		if b.Len() >= maxChars {
			break
		}
	}
	value := b.String()
	if len(value) > maxChars {
		value = truncateUTF8(value, maxChars)
	}
	if strings.TrimSpace(value) == "Earlier conversation summary:" {
		return fmt.Sprintf("Earlier conversation summary:\n- %d messages omitted.", len(facts))
	}
	return value
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func (p *plugin) compactOnce(
	ctx context.Context,
	contextID string,
	sessionID string,
	retainTokens int,
	maxChars int,
	reason string,
	view deriveOut,
	tools []toolDefinition,
) (compactOut, error) {
	out := compactOut{SessionID: sessionID}
	facts, err := p.queryMessages(ctx, contextID, sessionID)
	if err != nil {
		return out, err
	}
	ordinary := ordinaryFacts(facts)
	summarized, cutoff := selectSegments(ordinary, retainTokens)
	if cutoff == 0 || cutoff <= view.CoveredThroughSeq {
		return out, nil
	}

	summary := buildSummary(summarized, maxChars)
	sourceFrom := summarized[0].Seq
	compactionID := fmt.Sprintf("cc-%d-%d", time.Now().UnixNano(), p.seq.Add(1))
	tokenBefore := estimateTokens(toLLM(view.Messages), tools)
	tokenAfter := estimateAfterCompaction(view, summary, cutoff, tools)

	summaryPayload := summaryFactMeta{
		Active: true, CoversThroughSeq: cutoff,
		ShadowedFromSeq: sourceFrom, ShadowedToSeq: cutoff,
		PreviousSummary: view.SummarySeq,
		Plugin:          sourceModelContext, GeneratedBy: "model-context.compact",
		Strategy: "deterministic_digest", Reason: reason,
		CompactionID: compactionID, MessagesCovered: len(summarized),
	}
	compactionPayload := compactionFactMeta{
		Plugin: sourceModelContext, Reason: reason, Strategy: "deterministic_digest",
		CompactionID: compactionID, SourceFromSeq: sourceFrom, SourceToSeq: cutoff,
		Messages: len(summarized), TokenBefore: tokenBefore, TokenAfter: tokenAfter,
		PreviousSummary: view.SummarySeq,
	}
	appended, err := p.appendFacts(ctx, contextID, sessionID, []factIn{
		{Type: factTypeContextSummary, Role: roleHost, Content: summary, Meta: mustJSON(summaryPayload)},
		{Type: factTypeContextCompaction, Role: roleHost, Meta: mustJSON(compactionPayload)},
	})
	if err != nil {
		return out, err
	}
	summarySeq := 0
	if len(appended.Seqs) > 0 {
		summarySeq = appended.Seqs[0]
	}
	return compactOut{
		SessionID: sessionID, Compacted: true,
		SourceFromSeq: sourceFrom, SourceToSeq: cutoff, SummarySeq: summarySeq,
		Messages: len(summarized), TokenBefore: tokenBefore, TokenAfter: tokenAfter,
	}, nil
}
