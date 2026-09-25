package main

import "fmt"

// projectMessages is a pure projection: same facts → same deriveOut.
// Rules: active summary cutoff → message facts → last system in place
// → insert summary as system after last system (or head) → tool-result stub.
func projectMessages(facts []Fact, fullTool int) deriveOut {
	out := deriveOut{CoveredThroughSeq: 0}
	if fullTool < 0 {
		fullTool = 0
	}

	covered, summarySeq, summaryContent := 0, 0, ""
	for _, f := range facts {
		if f.Type != TypeContextSummary {
			continue
		}
		var sm summaryMeta
		if len(f.Meta) > 0 {
			_ = jsonUnmarshal(f.Meta, &sm)
		}
		if !sm.Active {
			continue
		}
		if f.Seq >= summarySeq {
			summarySeq = f.Seq
			covered = sm.CoversThroughSeq
			summaryContent = f.Content
		}
	}
	out.CoveredThroughSeq = covered
	out.SummarySeq = summarySeq

	msgs := make([]chatMessage, 0, len(facts))
	seqByMsg := make([]int, 0, len(facts))
	for _, f := range facts {
		if f.Type != TypeMessage || f.Seq <= covered {
			continue
		}
		msgs = append(msgs, chatMessage{
			Role:       f.Role,
			Content:    f.Content,
			ToolCalls:  f.ToolCalls,
			ToolCallID: f.ToolCallID,
		})
		seqByMsg = append(seqByMsg, f.Seq)
	}

	// Keep only the last role=system message, in place.
	lastSys := -1
	for i, m := range msgs {
		if m.Role == RoleSystem {
			lastSys = i
		}
	}
	if lastSys >= 0 {
		kept := msgs[:0]
		keptSeq := seqByMsg[:0]
		for i, m := range msgs {
			if m.Role == RoleSystem && i != lastSys {
				continue
			}
			kept = append(kept, m)
			keptSeq = append(keptSeq, seqByMsg[i])
		}
		msgs, seqByMsg = kept, keptSeq
		// lastSys index may have shifted if earlier systems were dropped before it
		lastSys = 0
		for i, m := range msgs {
			if m.Role == RoleSystem {
				lastSys = i
			}
		}
	}

	// Insert active summary as system after last system (or at head).
	if summarySeq > 0 {
		sum := chatMessage{Role: RoleSystem, Content: summaryContent}
		at := 0
		if lastSys >= 0 {
			at = lastSys + 1
		}
		msgs = append(msgs, chatMessage{})
		copy(msgs[at+1:], msgs[at:])
		msgs[at] = sum
		seqByMsg = append(seqByMsg, 0)
		copy(seqByMsg[at+1:], seqByMsg[at:])
		seqByMsg[at] = summarySeq
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
	}

	out.Messages = msgs
	return out
}
