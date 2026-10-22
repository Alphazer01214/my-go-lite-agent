package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (p *plugin) newSnapshot(meta completeMeta, view deriveOut, messages []llmMessage, tools []toolDefinition, warnings []string) snapshot {
	if tools == nil {
		tools = []toolDefinition{}
	}
	revision := int(p.revision.Add(1))
	return snapshot{
		ModelContextID: fmt.Sprintf("mc-%d-%d", time.Now().UnixNano(), revision),
		Revision:       revision,
		SessionID:      meta.SessionID,
		TurnID:         meta.TurnID,
		StepID:         meta.StepID,
		Mode:           meta.Mode,
		Model:          meta.Model,
		ContextWindow:  meta.ContextWindow,
		Surface:        snapshotSurface(view.Messages),
		MessageCount:   len(messages),
		MessagesHash:   messagesHash(messages),
		TruncatedSeqs:  append([]int(nil), view.TruncatedSeqs...),
		Tools:          tools,
		ToolsHash:      toolsHash(tools),
		TokenEstimate:  estimateTokens(messages, tools),
		Parts:          snapshotParts(view),
		Warnings:       dedupeStrings(warnings),
	}
}

func snapshotParts(view deriveOut) []snapshotPart {
	parts := make([]snapshotPart, 0, 4)
	var prompt, runtime, summary, session []int
	for _, message := range view.Messages {
		meta := parseDerivedMeta(message.Meta)
		switch {
		case meta.Kind == kindSystemPrompt:
			prompt = append(prompt, message.Seq)
		case meta.Kind == kindRuntimeContext:
			runtime = append(runtime, message.Seq)
		case view.SummarySeq != 0 && message.Seq == view.SummarySeq:
			summary = append(summary, message.Seq)
		default:
			session = append(session, message.Seq)
		}
	}
	if len(prompt) > 0 {
		parts = append(parts, snapshotPart{Source: "system_prompt", Seqs: prompt})
	}
	if len(summary) > 0 {
		parts = append(parts, snapshotPart{Source: "summary", Seqs: summary})
	}
	if len(runtime) > 0 {
		parts = append(parts, snapshotPart{Source: "runtime_context", Seqs: runtime})
	}
	if len(session) > 0 {
		parts = append(parts, snapshotPart{Source: "session", Seqs: session})
	}
	return parts
}

func (p *plugin) persistSnapshot(ctx context.Context, contextID string, snap snapshot) error {
	_, err := p.appendFacts(ctx, contextID, snap.SessionID, []factIn{{
		Type: factTypeContextSnapshot, Role: roleHost, Meta: mustJSON(snap),
	}})
	return err
}

func (p *plugin) rememberSnapshot(snap snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshots[snap.ModelContextID] = snap
	p.snapshotOrder = append(p.snapshotOrder, snap.ModelContextID)
	limit := p.cfg.SnapshotMemoryLimit
	if limit <= 0 {
		limit = defaultSnapshotLimit
	}
	for len(p.snapshotOrder) > limit {
		oldest := p.snapshotOrder[0]
		p.snapshotOrder = p.snapshotOrder[1:]
		delete(p.snapshots, oldest)
	}
}

func latestSnapshotFromFacts(facts []fact) (snapshot, bool) {
	var latest *fact
	for i := range facts {
		if facts[i].Type != factTypeContextSnapshot {
			continue
		}
		if latest == nil || facts[i].Seq > latest.Seq {
			latest = &facts[i]
		}
	}
	if latest == nil {
		return snapshot{}, false
	}
	var snap snapshot
	if len(latest.Meta) > 0 {
		_ = json.Unmarshal(latest.Meta, &snap)
	}
	if snap.ModelContextID == "" && len(snap.Surface) == 0 {
		return snapshot{}, false
	}
	return snap, true
}

func reconstructSnapshot(snap snapshot, facts []fact) ([]inspectedMessage, inspectBreakdown, []string, string) {
	bySeq := make(map[int]fact, len(facts))
	for _, item := range facts {
		if item.Type == factTypeMessage || item.Type == factTypeContextSummary {
			bySeq[item.Seq] = item
		}
	}
	truncated := make(map[int]bool, len(snap.TruncatedSeqs))
	for _, seq := range snap.TruncatedSeqs {
		truncated[seq] = true
	}
	messages := make([]inspectedMessage, 0, len(snap.Surface))
	llmMessages := make([]llmMessage, 0, len(snap.Surface))
	var breakdown inspectBreakdown
	warnings := make([]string, 0)
	integrity := "ok"
	for _, seq := range snap.Surface {
		item, ok := bySeq[seq]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("surface seq %d not found", seq))
			integrity = "mismatch"
			continue
		}
		role := item.Role
		source := "session"
		if item.Type == factTypeContextSummary {
			role = roleSystem
			source = "summary"
		} else {
			meta := parseDerivedMeta(item.Meta)
			switch meta.Kind {
			case kindSystemPrompt:
				source = "system_prompt"
			case kindRuntimeContext:
				source = "runtime_context"
			}
		}
		content := item.Content
		isTruncated := truncated[seq]
		if isTruncated {
			content = fmt.Sprintf("[tool result omitted; %d bytes; see seq=%d]", len(item.Content), item.Seq)
		}
		message := inspectedMessage{
			Seq: seq, Role: role, Content: content,
			ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID,
			Source: source, Truncated: isTruncated,
		}
		message.TokenEstimate = estimateMessageTokens(llmMessage{
			Role: role, Content: content, ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID,
		})
		switch source {
		case "system_prompt":
			breakdown.SystemPrompt += message.TokenEstimate
		case "runtime_context":
			breakdown.RuntimeContext += message.TokenEstimate
		case "summary":
			breakdown.Summary += message.TokenEstimate
		default:
			breakdown.Session += message.TokenEstimate
		}
		messages = append(messages, message)
	}
	// Rebuild the hashed form from the repaired messages: snapshots of older
	// sessions were recorded after the same orphan-tool repair in derive.
	repairInspectedToolMessages(messages)
	llmMessages = llmMessages[:0]
	for _, message := range messages {
		llmMessages = append(llmMessages, llmMessage{
			Role: message.Role, Content: message.Content,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		})
	}
	breakdown.Tools = estimateToolsTokens(snap.Tools)
	if snap.MessagesHash != "" && messagesHash(llmMessages) != snap.MessagesHash {
		warnings = append(warnings, "messages_hash_mismatch")
		integrity = "mismatch"
	}
	return messages, breakdown, warnings, integrity
}

func repairInspectedToolMessages(messages []inspectedMessage) {
	pending := map[string]bool{}
	for i := range messages {
		message := &messages[i]
		switch message.Role {
		case roleAssistant:
			pending = toolCallIDs(message.ToolCalls)
		case roleTool:
			if message.ToolCallID != "" && pending[message.ToolCallID] {
				delete(pending, message.ToolCallID)
				continue
			}
			prefix := fmt.Sprintf("[recovered tool result; original tool_call_id=%q]\n", message.ToolCallID)
			message.Content = strings.TrimRight(prefix+message.Content, "\n")
			message.Role = roleUser
			message.ToolCalls = nil
			message.ToolCallID = ""
		default:
			pending = map[string]bool{}
		}
	}
}

func toolCallIDs(raw json.RawMessage) map[string]bool {
	pending := map[string]bool{}
	if len(raw) == 0 {
		return pending
	}
	var calls []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return pending
	}
	for _, call := range calls {
		if call.ID != "" {
			pending[call.ID] = true
		}
	}
	return pending
}

func (p *plugin) latestSnapshotTools(sessionID string) []toolDefinition {
	p.mu.Lock()
	defer p.mu.Unlock()
	var best snapshot
	found := false
	for _, snap := range p.snapshots {
		if snap.SessionID != sessionID {
			continue
		}
		if !found || snap.Revision > best.Revision {
			best = snap
			found = true
		}
	}
	if !found {
		return []toolDefinition{}
	}
	return append([]toolDefinition(nil), best.Tools...)
}
