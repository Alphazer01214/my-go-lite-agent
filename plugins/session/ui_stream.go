package main

import (
	"context"
	"strings"
)

func (p *plugin) updateStream(ctx context.Context, in streamUpdateIn) (streamUpdateOut, error) {
	if in.SessionID == "" || in.TurnID == "" || in.StepID == "" {
		return streamUpdateOut{}, errBadArguments("session_id, turn_id and step_id are required")
	}
	if in.UpdateID <= 0 {
		return streamUpdateOut{}, errBadArguments("update_id must be positive")
	}
	if in.Status == "" {
		in.Status = "streaming"
	}
	if _, err := p.get(in.SessionID); err != nil {
		return streamUpdateOut{}, err
	}

	p.mu.Lock()
	if p.streams == nil {
		p.streams = make(map[string]streamSnapshot)
	}
	previous, ok := p.streams[in.SessionID]
	if ok && previous.TurnID == in.TurnID && previous.StepID == in.StepID && in.UpdateID <= previous.UpdateID {
		p.mu.Unlock()
		return streamUpdateOut{Accepted: false, Reason: "stale"}, nil
	}
	reset := true
	contentDelta := in.Message.Content
	reasoningDelta := in.Message.Reasoning
	if ok && previous.TurnID == in.TurnID && previous.StepID == in.StepID &&
		strings.HasPrefix(in.Message.Content, previous.Message.Content) &&
		strings.HasPrefix(in.Message.Reasoning, previous.Message.Reasoning) {
		reset = false
		contentDelta = in.Message.Content[len(previous.Message.Content):]
		reasoningDelta = in.Message.Reasoning[len(previous.Message.Reasoning):]
	}
	snapshot := streamSnapshot{
		SessionID: in.SessionID,
		TurnID:    in.TurnID,
		StepID:    in.StepID,
		UpdateID:  in.UpdateID,
		Message:   in.Message,
		Status:    in.Status,
	}
	p.streams[in.SessionID] = snapshot
	open := p.isOpenLocked(in.SessionID)
	p.mu.Unlock()

	// Every open pane tracks its own stream, including background sessions.
	// The outbox coalesces deltas; the handler never waits for a UI round trip.
	if open {
		p.enqueueStream(streamDeltaUI{
			SessionID: in.SessionID, TurnID: in.TurnID, StepID: in.StepID,
			UpdateID: in.UpdateID, Reset: reset,
			ContentDelta: contentDelta, ReasoningDelta: reasoningDelta, Status: in.Status,
		})
	}
	if in.Status == "cancel_requested" {
		p.publishPanes(ctx)
	}
	return streamUpdateOut{Accepted: true}, nil
}

func (p *plugin) clearStream(sessionID, turnID, stepID string) {
	p.mu.Lock()
	if snapshot, ok := p.streams[sessionID]; ok &&
		(turnID == "" || snapshot.TurnID == turnID) &&
		(stepID == "" || snapshot.StepID == stepID) {
		delete(p.streams, sessionID)
	}
	p.mu.Unlock()
	p.dropStream(sessionID, turnID, stepID)
}
