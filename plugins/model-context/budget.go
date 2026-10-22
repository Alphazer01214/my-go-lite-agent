package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

func (p *plugin) contextLimit(contextWindow int) int {
	limit := p.cfg.MaxContextTokens
	if contextWindow > 0 && (limit <= 0 || contextWindow < limit) {
		return contextWindow
	}
	return limit
}

func (p *plugin) enforceBudget(
	ctx context.Context,
	contextID string,
	meta completeMeta,
	tools []toolDefinition,
	view deriveOut,
) (deriveOut, []string, error) {
	limit := p.contextLimit(meta.ContextWindow)
	if limit <= 0 {
		return view, nil, nil
	}
	estimate := estimateTokens(toLLM(view.Messages), tools)
	trigger := int(float64(limit) * p.cfg.CompactThreshold)
	if estimate < trigger {
		return view, nil, nil
	}

	warnings := make([]string, 0, 1)
	for range 3 {
		out, err := p.compactOnce(ctx, contextID, meta.SessionID, p.cfg.RetainRecentTokens, p.cfg.CompactMaxChars, "budget", view, tools)
		if err != nil {
			return view, warnings, err
		}
		if !out.Compacted {
			break
		}
		if !containsString(warnings, "compacted") {
			warnings = append(warnings, "compacted")
		}
		view, err = p.derive(ctx, contextID, meta.SessionID)
		if err != nil {
			return view, warnings, err
		}
		estimate = estimateTokens(toLLM(view.Messages), tools)
		if estimate < limit {
			return view, warnings, nil
		}
	}
	if estimate >= limit {
		return view, warnings, errCoded(codeContextBudgetExceeded,
			fmt.Sprintf("context estimate %d exceeds max_context_tokens %d", estimate, limit))
	}
	return view, warnings, nil
}

type segment struct {
	facts  []fact
	tokens int
}

func selectSegments(ordinary []fact, retainTokens int) ([]fact, int) {
	if len(ordinary) == 0 {
		return nil, 0
	}
	segments := make([]segment, 0, len(ordinary))
	current := segment{}
	for _, item := range ordinary {
		if item.Role == roleUser && len(current.facts) > 0 {
			segments = append(segments, current)
			current = segment{}
		}
		current.facts = append(current.facts, item)
		current.tokens += estimateFactTokens(item)
	}
	if len(current.facts) > 0 {
		segments = append(segments, current)
	}

	keepFrom := len(segments)
	retained := 0
	for i := len(segments) - 1; i >= 0; i-- {
		if i < len(segments)-1 && retained+segments[i].tokens > retainTokens {
			break
		}
		keepFrom = i
		retained += segments[i].tokens
		if retained >= retainTokens {
			break
		}
	}
	if keepFrom == 0 {
		return nil, 0
	}
	summarized := make([]fact, 0)
	for _, seg := range segments[:keepFrom] {
		summarized = append(summarized, seg.facts...)
	}
	if len(summarized) == 0 {
		return nil, 0
	}
	return summarized, summarized[len(summarized)-1].Seq
}

func estimateFactTokens(item fact) int {
	return estimateMessageTokens(llmMessage{
		Role: item.Role, Content: item.Content, ToolCalls: item.ToolCalls,
	})
}

func ordinaryFacts(facts []fact) []fact {
	out := make([]fact, 0, len(facts))
	for _, item := range facts {
		if item.Type != factTypeMessage || item.Role == roleSystem {
			continue
		}
		meta := parseDerivedMeta(item.Meta)
		if meta.Kind == kindSystemPrompt || meta.Kind == kindRuntimeContext {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

func estimateAfterCompaction(view deriveOut, summary string, cutoff int, tools []toolDefinition) int {
	messages := make([]llmMessage, 0, len(view.Messages)+1)
	inserted := false
	for _, message := range view.Messages {
		if view.SummarySeq != 0 && message.Seq == view.SummarySeq {
			continue
		}
		if message.Role != roleSystem && message.Seq <= cutoff {
			continue
		}
		messages = append(messages, llmMessage{
			Role: message.Role, Content: message.Content,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		})
		if message.Role == roleSystem && !inserted {
			messages = append(messages, llmMessage{Role: roleSystem, Content: summary})
			inserted = true
		}
	}
	if !inserted {
		messages = append([]llmMessage{{Role: roleSystem, Content: summary}}, messages...)
	}
	return estimateTokens(messages, tools)
}

func estimateToolsTokens(tools []toolDefinition) int {
	if len(tools) == 0 {
		return 0
	}
	body, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return (len(body) + 3) / 4
}
