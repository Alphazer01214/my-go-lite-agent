package main

import (
	"context"
)

func (p *plugin) cancel(ctx context.Context, in cancelIn) (cancelOut, error) {
	if in.SessionID == "" {
		return cancelOut{}, errBadArguments("session_id is required")
	}
	slot := p.slotFor(in.SessionID)
	slot.mu.Lock()
	active := slot.active
	slot.mu.Unlock()
	if active == nil {
		return cancelOut{SessionID: in.SessionID, Cancelled: false}, nil
	}
	active.markCancelled()
	callID, turnID, stepID := active.cancelContext()
	if callID != "" {
		_, _ = p.sdk.Call(ctx, "llm", "cancel", mustJSON(llmCancelIn{CallID: callID}))
	}
	if turnID != "" && stepID != "" {
		_ = p.updateStream(ctx, streamSnapshot{
			SessionID: in.SessionID,
			TurnID:    turnID,
			StepID:    stepID,
			UpdateID:  active.nextUpdateID(),
			Message:   streamMessage{Role: RoleAssistant},
			Status:    "cancel_requested",
		})
	}
	return cancelOut{SessionID: in.SessionID, Cancelled: true}, nil
}
