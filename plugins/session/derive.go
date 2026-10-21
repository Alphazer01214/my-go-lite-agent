package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// projectMessages is a pure projection: same facts → same deriveOut.
//
// Model-visible history is a view over an append-only log:
//   - the latest model-context system prompt is the head system message;
//   - the latest active summary shadows an ordinary-message range and is
//     inserted as a system message;
//   - the latest runtime-context snapshot is inserted after the summary;
//   - ordinary messages outside the shadowed range follow in seq order;
//   - old tool results are stubbed deterministically.
//
// system_prompt and runtime_context facts are exempt from shadowing: they are
// current state, not conversation history.
func projectMessages(facts []Fact, fullTool int) deriveOut {
	out := deriveOut{CoveredThroughSeq: 0}
	if fullTool < 0 {
		fullTool = 0
	}

	var prompt, runtime, fallbackSystem *Fact
	var summary summaryMeta
	summarySeq := 0
	for i := range facts {
		f := &facts[i]
		if f.Type == TypeContextSummary {
			var sm summaryMeta
			if len(f.Meta) > 0 {
				_ = json.Unmarshal(f.Meta, &sm)
			}
			if !sm.Active {
				continue
			}
			to := sm.ShadowedToSeq
			if to == 0 {
				to = sm.CoversThroughSeq
			}
			if summarySeq == 0 || f.Seq > summarySeq {
				summarySeq = f.Seq
				summary = sm
				summary.ShadowedToSeq = to
			}
			continue
		}
		if f.Type != TypeMessage {
			continue
		}
		var mm messageMeta
		if len(f.Meta) > 0 {
			_ = json.Unmarshal(f.Meta, &mm)
		}
		switch mm.Kind {
		case kindSystemPrompt:
			if prompt == nil || f.Seq > prompt.Seq {
				prompt = f
			}
		case kindRuntimeContext:
			if runtime == nil || f.Seq > runtime.Seq {
				runtime = f
			}
		}
	}

	shadowed := func(seq int) bool {
		if summarySeq == 0 || seq > summary.ShadowedToSeq {
			return false
		}
		return summary.ShadowedFromSeq == 0 || seq >= summary.ShadowedFromSeq
	}

	// Without a model-context prompt, keep the old rule: the latest visible
	// system message is the head. It still participates in shadowing.
	if prompt == nil {
		for i := range facts {
			f := &facts[i]
			if f.Type != TypeMessage || f.Role != RoleSystem {
				continue
			}
			var mm messageMeta
			if len(f.Meta) > 0 {
				_ = json.Unmarshal(f.Meta, &mm)
			}
			if mm.Kind == kindSystemPrompt || mm.Kind == kindRuntimeContext {
				continue
			}
			if shadowed(f.Seq) {
				continue
			}
			if fallbackSystem == nil || f.Seq > fallbackSystem.Seq {
				fallbackSystem = f
			}
		}
	}

	out.CoveredThroughSeq = summary.ShadowedToSeq
	out.SummarySeq = summarySeq

	msgs := make([]chatMessage, 0, len(facts))
	seqByMsg := make([]int, 0, len(facts))
	add := func(seq int, role, content string, toolCalls json.RawMessage, toolCallID string, meta json.RawMessage) {
		msgs = append(msgs, chatMessage{
			Seq: seq, Role: role, Content: content,
			ToolCalls: toolCalls, ToolCallID: toolCallID, Meta: cloneMeta(meta),
		})
		seqByMsg = append(seqByMsg, seq)
	}

	head := prompt
	if head == nil {
		head = fallbackSystem
	}
	if head != nil {
		add(head.Seq, RoleSystem, head.Content, nil, "", head.Meta)
	}
	if summarySeq != 0 {
		var summaryFact *Fact
		for i := range facts {
			if facts[i].Type == TypeContextSummary && facts[i].Seq == summarySeq {
				summaryFact = &facts[i]
				break
			}
		}
		if summaryFact != nil {
			add(summaryFact.Seq, RoleSystem, summaryFact.Content, nil, "", summaryFact.Meta)
		}
	}
	if runtime != nil {
		add(runtime.Seq, runtime.Role, runtime.Content, nil, "", runtime.Meta)
	}

	for i := range facts {
		f := &facts[i]
		if f.Type != TypeMessage || f.Role == RoleSystem {
			continue
		}
		var mm messageMeta
		if len(f.Meta) > 0 {
			_ = json.Unmarshal(f.Meta, &mm)
		}
		if mm.Kind == kindSystemPrompt || mm.Kind == kindRuntimeContext {
			continue
		}
		if shadowed(f.Seq) {
			continue
		}
		add(f.Seq, f.Role, f.Content, f.ToolCalls, f.ToolCallID, f.Meta)
	}

	// Tool-result stub: last fullTool role=tool keep text; earlier ones stubbed.
	toolIdx := make([]int, 0)
	for i, m := range msgs {
		if m.Role == RoleTool {
			toolIdx = append(toolIdx, i)
		}
	}
	dropBefore := len(toolIdx) - fullTool
	if dropBefore < 0 {
		dropBefore = 0
	}
	for j, i := range toolIdx {
		if j >= dropBefore {
			continue
		}
		msgs[i].Content = fmt.Sprintf("[tool result omitted; %d bytes; see seq=%d]", len(msgs[i].Content), seqByMsg[i])
		out.TruncatedTools++
		out.TruncatedSeqs = append(out.TruncatedSeqs, seqByMsg[i])
	}

	repairOrphanToolMessages(msgs)
	out.Messages = msgs
	return out
}

// repairOrphanToolMessages keeps the projected history valid for providers that
// reject role=tool without a preceding assistant message listing that
// tool_call_id. Older logs persisted tool results but dropped the assistant
// tool_calls; those results become user messages so their content survives
// without fabricating a tool call.
func repairOrphanToolMessages(msgs []chatMessage) {
	pending := map[string]bool{}
	for i := range msgs {
		message := &msgs[i]
		switch message.Role {
		case RoleAssistant:
			pending = toolCallIDs(message.ToolCalls)
		case RoleTool:
			if message.ToolCallID != "" && pending[message.ToolCallID] {
				delete(pending, message.ToolCallID)
				continue
			}
			prefix := fmt.Sprintf("[recovered tool result; original tool_call_id=%q]\n", message.ToolCallID)
			message.Content = strings.TrimRight(prefix+message.Content, "\n")
			message.Role = RoleUser
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

func cloneMeta(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
