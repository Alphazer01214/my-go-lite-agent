package main

import (
	"context"
	"encoding/json"
	"sort"
)

func windowChatMessages(messages []chatMessageUI) ([]chatMessageUI, int, bool) {
	total := len(messages)
	start := total
	bytes := 0
	for start > 0 && total-start < chatWindowMessages {
		message := messages[start-1]
		size := len(message.Content) + len(message.Reasoning) + len(message.ToolCalls)
		if start < total && bytes+size > chatWindowBytes {
			break
		}
		bytes += size
		start--
	}
	window := messages[start:]
	firstSeq := 0
	if len(window) > 0 {
		firstSeq = window[0].Seq
	}
	return window, firstSeq, start > 0
}

type subagentFactMeta struct {
	ChildSessionID string `json:"child_session_id"`
	State          string `json:"state"`
	StartedTS      int64  `json:"started_ts,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
	TurnCount      int    `json:"turn_count,omitempty"`
	Error          string `json:"error,omitempty"`
}

func (p *plugin) subagentsForSession(sessionID string) []subagentUI {
	facts, _, err := p.readFacts(sessionID)
	if err != nil {
		return nil
	}
	return subagentsFromFacts(facts)
}

func subagentsFromFacts(facts []Fact) []subagentUI {
	byID := make(map[string]*subagentUI)
	order := make([]string, 0, 2)
	for _, fact := range facts {
		if fact.Type != TypeSubagentSpawned && fact.Type != TypeSubagentFinished {
			continue
		}
		var meta subagentFactMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		if meta.ChildSessionID == "" {
			continue
		}
		entry := byID[meta.ChildSessionID]
		if entry == nil {
			entry = &subagentUI{
				ChildSessionID: meta.ChildSessionID, State: "running",
				StartedTS: meta.StartedTS,
			}
			if entry.StartedTS == 0 {
				entry.StartedTS = fact.Timestamp
			}
			byID[meta.ChildSessionID] = entry
			order = append(order, meta.ChildSessionID)
		}
		if fact.Type == TypeSubagentFinished {
			entry.State = meta.State
			entry.EndedTS = fact.Timestamp
			entry.DurationMS = meta.DurationMS
			entry.TurnCount = meta.TurnCount
			entry.Error = meta.Error
		}
	}
	out := make([]subagentUI, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedTS != out[j].StartedTS {
			return out[i].StartedTS < out[j].StartedTS
		}
		return out[i].ChildSessionID < out[j].ChildSessionID
	})
	return out
}

func chatMessageFromFact(fact Fact) chatMessageUI {
	var meta factContextMeta
	if len(fact.Meta) > 0 {
		_ = json.Unmarshal(fact.Meta, &meta)
	}
	return chatMessageUI{
		Seq: fact.Seq, TurnID: meta.TurnID, StepID: meta.StepID,
		Role: fact.Role, Content: fact.Content, Reasoning: meta.Reasoning,
		Timestamp: fact.Timestamp, ToolCalls: fact.ToolCalls, ToolCallID: fact.ToolCallID,
		ToolName: meta.ToolName, IsError: meta.IsError, Truncated: meta.Truncated,
		Source: meta.Source, Kind: meta.Kind,
	}
}

func turnAppendFromFacts(facts []Fact) *chatTurnAppendUI {
	var turn *chatTurnAppendUI
	for _, fact := range facts {
		var meta factContextMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		switch fact.Type {
		case TypeTurnStart:
			turn = &chatTurnAppendUI{TurnID: meta.TurnID, Status: "running"}
		case TypeStepStart:
			if turn == nil {
				turn = &chatTurnAppendUI{TurnID: meta.TurnID}
			}
			turn.TurnID = meta.TurnID
			turn.Status = "running"
			turn.Step = meta.Step
		case TypeTurnEnd:
			if turn == nil {
				turn = &chatTurnAppendUI{TurnID: meta.TurnID}
			}
			turn.TurnID = meta.TurnID
			turn.Status = meta.Reason
			turn.Error = meta.Error
		}
	}
	return turn
}

func contextFromSnapshotMeta(meta json.RawMessage) *chatContextUI {
	var snap struct {
		ModelContextID string   `json:"model_context_id"`
		TurnID         string   `json:"turn_id"`
		StepID         string   `json:"step_id"`
		MessageCount   int      `json:"message_count"`
		TokenEstimate  int      `json:"token_estimate"`
		Model          string   `json:"model"`
		ContextWindow  int      `json:"context_window"`
		Warnings       []string `json:"warnings"`
	}
	_ = json.Unmarshal(meta, &snap)
	return &chatContextUI{
		ModelContextID: snap.ModelContextID,
		TurnID:         snap.TurnID,
		StepID:         snap.StepID,
		MessageCount:   snap.MessageCount,
		TokenEstimate:  snap.TokenEstimate,
		Model:          snap.Model,
		ContextWindow:  snap.ContextWindow,
		Warnings:       snap.Warnings,
	}
}

func requestFromHeaderMeta(meta json.RawMessage) *chatRequestUI {
	var rm struct {
		TurnID        string `json:"turn_id"`
		StepID        string `json:"step_id"`
		Mode          string `json:"mode"`
		Scheme        string `json:"scheme"`
		Model         string `json:"model"`
		ContextWindow int    `json:"context_window"`
	}
	_ = json.Unmarshal(meta, &rm)
	return &chatRequestUI{
		TurnID: rm.TurnID, StepID: rm.StepID,
		Mode: rm.Mode, Scheme: rm.Scheme,
		Model: rm.Model, ContextWindow: rm.ContextWindow,
	}
}

func (p *plugin) chatState(ctx context.Context, sessionID string) (chatStateUI, error) {
	return p.chatStateInternal(ctx, sessionID, true)
}

func (p *plugin) chatStateInternal(ctx context.Context, sessionID string, allowRecovery bool) (chatStateUI, error) {
	st, err := p.get(sessionID)
	if err != nil {
		return chatStateUI{}, err
	}
	meta := st.snapshotMeta()
	facts, _, err := p.readFacts(sessionID)
	if err != nil {
		return chatStateUI{}, err
	}
	turnOwner := ""
	state := chatStateUI{
		SessionID: sessionID, Name: meta.Name,
		WorkspaceID: meta.WorkspaceID, Workspace: meta.Workspace,
		Model: meta.Model, ContextWindow: meta.ContextWindow,
		Origin: meta.Origin, ParentSessionID: meta.ParentSessionID,
		ReadOnly:  meta.Origin == "subagent",
		Subagents: subagentsFromFacts(facts),
		Messages:  make([]chatMessageUI, 0, len(facts)),
		Turn:      chatTurnUI{Status: "idle"},
	}
	var usage chatUsageUI
	var latestUsageModel string
	var latestUsageWindow int
	var contextMeta json.RawMessage
	var requestMeta json.RawMessage
	turnInfos := make(map[string]*chatTurnInfoUI)
	turnOrder := make([]string, 0)
	ensureTurn := func(id string) *chatTurnInfoUI {
		info, ok := turnInfos[id]
		if !ok {
			info = &chatTurnInfoUI{TurnID: id, Status: "running"}
			turnInfos[id] = info
			turnOrder = append(turnOrder, id)
		}
		return info
	}
	for _, fact := range facts {
		var meta factContextMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		switch fact.Type {
		case TypeMessage:
			if meta.Kind == "turn_cancelled" {
				break
			}
			state.Messages = append(state.Messages, chatMessageUI{
				Seq: fact.Seq, TurnID: meta.TurnID, StepID: meta.StepID,
				Role: fact.Role, Content: fact.Content, Reasoning: meta.Reasoning,
				Timestamp: fact.Timestamp, ToolCalls: fact.ToolCalls, ToolCallID: fact.ToolCallID,
				ToolName: meta.ToolName, IsError: meta.IsError, Truncated: meta.Truncated,
				Source: meta.Source, Kind: meta.Kind,
			})
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Messages++
				if fact.Role == RoleTool {
					info.Tools++
				}
			}
		case TypeTurnStart:
			state.Turn = chatTurnUI{TurnID: meta.TurnID, Status: "running"}
			turnOwner = meta.Plugin
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Status = "running"
				if info.StartedAt == 0 {
					info.StartedAt = fact.Timestamp
				}
			}
		case TypeStepStart:
			state.Turn.TurnID = meta.TurnID
			state.Turn.Step = meta.Step
			state.Turn.Status = "running"
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Status = "running"
				info.Step = meta.Step
				if info.StartedAt == 0 {
					info.StartedAt = fact.Timestamp
				}
			}
		case TypeTurnEnd:
			state.Turn.TurnID = meta.TurnID
			state.Turn.Status = meta.Reason
			state.Turn.Error = meta.Error
			if meta.TurnID != "" {
				info := ensureTurn(meta.TurnID)
				info.Status = meta.Reason
				info.EndedAt = fact.Timestamp
				info.Error = meta.Error
			}
		case TypeLLMUsage:
			var um struct {
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
				Model         string `json:"model"`
				ContextWindow int    `json:"context_window"`
			}
			if len(fact.Meta) > 0 {
				_ = json.Unmarshal(fact.Meta, &um)
			}
			total := um.Usage.TotalTokens
			if total == 0 {
				total = um.Usage.PromptTokens + um.Usage.CompletionTokens
			}
			usage.PromptTokens += um.Usage.PromptTokens
			usage.CompletionTokens += um.Usage.CompletionTokens
			usage.TotalTokens += total
			usage.Requests++
			usage.LastPromptTokens = um.Usage.PromptTokens
			if um.Model != "" {
				latestUsageModel = um.Model
			}
			if um.ContextWindow > 0 {
				latestUsageWindow = um.ContextWindow
			}
		case TypeContextSnapshot:
			// Keep only the newest raw meta. A snapshot carries the full tool
			// list, so reparsing every historical snapshot would dominate a
			// long session's projection cost.
			contextMeta = fact.Meta
		case TypeRequestHeader:
			requestMeta = fact.Meta
		}
	}
	var contextSummary *chatContextUI
	if len(contextMeta) > 0 {
		var snap struct {
			ModelContextID string   `json:"model_context_id"`
			TurnID         string   `json:"turn_id"`
			StepID         string   `json:"step_id"`
			MessageCount   int      `json:"message_count"`
			TokenEstimate  int      `json:"token_estimate"`
			Model          string   `json:"model"`
			ContextWindow  int      `json:"context_window"`
			Warnings       []string `json:"warnings"`
		}
		_ = json.Unmarshal(contextMeta, &snap)
		contextSummary = &chatContextUI{
			ModelContextID: snap.ModelContextID,
			TurnID:         snap.TurnID,
			StepID:         snap.StepID,
			MessageCount:   snap.MessageCount,
			TokenEstimate:  snap.TokenEstimate,
			Model:          snap.Model,
			ContextWindow:  snap.ContextWindow,
			Warnings:       snap.Warnings,
		}
	}
	var requestSummary *chatRequestUI
	if len(requestMeta) > 0 {
		var rm struct {
			TurnID        string `json:"turn_id"`
			StepID        string `json:"step_id"`
			Mode          string `json:"mode"`
			Scheme        string `json:"scheme"`
			Model         string `json:"model"`
			ContextWindow int    `json:"context_window"`
		}
		_ = json.Unmarshal(requestMeta, &rm)
		requestSummary = &chatRequestUI{
			TurnID: rm.TurnID, StepID: rm.StepID,
			Mode: rm.Mode, Scheme: rm.Scheme,
			Model: rm.Model, ContextWindow: rm.ContextWindow,
		}
	}
	state.Turns = make([]chatTurnInfoUI, 0, len(turnOrder))
	for _, id := range turnOrder {
		state.Turns = append(state.Turns, *turnInfos[id])
	}
	usage.Model = latestUsageModel
	usage.ContextWindow = latestUsageWindow
	state.Usage = usage
	state.Context = contextSummary
	state.Request = requestSummary
	if state.Model == "" {
		state.Model = latestUsageModel
		if state.Model == "" && requestSummary != nil {
			state.Model = requestSummary.Model
		}
	}
	// The meter's numerator and denominator must come from the same request.
	// Session metadata is only a fallback before the first usage arrives.
	if latestUsageWindow > 0 {
		state.ContextWindow = latestUsageWindow
	}
	if state.ContextWindow <= 0 && contextSummary != nil {
		state.ContextWindow = contextSummary.ContextWindow
	}
	if state.ContextWindow <= 0 && requestSummary != nil {
		state.ContextWindow = requestSummary.ContextWindow
	}
	if state.Messages == nil {
		state.Messages = []chatMessageUI{}
	}
	state.Messages, state.FirstSeq, state.HasMore = windowChatMessages(state.Messages)
	ownerState := p.pluginState(ctx, turnOwner)
	if allowRecovery && state.Turn.Status == "running" && turnOwner != "" && turnOwnerUnavailable(ownerState) {
		if p.recoverOpenTurn(sessionID, state.Turn.TurnID, turnOwner) {
			return p.chatStateInternal(ctx, sessionID, false)
		}
	}
	p.mu.RLock()
	stream, streamOK := p.streams[sessionID]
	p.mu.RUnlock()
	if streamOK {
		copy := stream
		state.Stream = &copy
	}
	if streamOK && stream.Status == "cancel_requested" && stream.TurnID == state.Turn.TurnID {
		state.Turn.CancelRequested = true
	}
	state.ThroughSeq = lastFactSeq(facts)
	state.Revision = p.nextUIRevision()
	return state, nil
}

func turnOwnerUnavailable(state string) bool {
	switch state {
	case "failed", "inactive", "stopping", "missing":
		return true
	default:
		return false
	}
}
