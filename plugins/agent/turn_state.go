package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

type turnState struct {
	ctx          context.Context
	sessionID    string
	turnID       string
	stepID       string
	activeCallID string
	streamSeq    int64
	mu           sync.Mutex
	cancelled    bool
	done         chan struct{}
}

func newTurnState(ctx context.Context, id string) *turnState {
	return &turnState{ctx: ctx, sessionID: id, done: make(chan struct{})}
}

func (t *turnState) isCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cancelled
}

func (t *turnState) markCancelled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled {
		return false
	}
	t.cancelled = true
	close(t.done)
	return true
}

func (t *turnState) setTurnID(id string) {
	t.mu.Lock()
	t.turnID = id
	t.mu.Unlock()
}

func (t *turnState) setStepID(id string) {
	t.mu.Lock()
	t.stepID = id
	t.mu.Unlock()
}

func (t *turnState) beginCall(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelled {
		return false
	}
	t.activeCallID = id
	return true
}

func (t *turnState) endCall(id string) {
	t.mu.Lock()
	if t.activeCallID == id {
		t.activeCallID = ""
	}
	t.mu.Unlock()
}

func (t *turnState) cancelContext() (callID, turnID, stepID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.activeCallID, t.turnID, t.stepID
}

func (t *turnState) withNextUpdate(snapshot streamSnapshot) streamSnapshot {
	snapshot.UpdateID = t.nextUpdateID()
	return snapshot
}

func (t *turnState) nextUpdateID() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.streamSeq++
	return int(t.streamSeq)
}

type emitFunc func(payload json.RawMessage) error

// turnFinalizeTimeout bounds the terminal writes that must survive the handler
// context being canceled. It is short because these writes are local calls.
const turnFinalizeTimeout = 2 * time.Second

// cancelledTurnNote is recorded as a user message after a cancelled turn. It
// stays in the model-visible log so the next turn does not resume the abandoned
// task by accident. The chat UI filters this meta.kind.
const cancelledTurnNote = "The user cancelled the previous turn before it completed. Do not continue that task unless the user explicitly asks again."

// cancelledToolResult closes a tool_call id whose execution never started. Every
// assistant tool_calls entry needs a matching tool result in the next request.
const cancelledToolResult = "tool call not executed: the turn was cancelled before this tool ran"

// finalizeContext detaches terminal bookkeeping from the caller's cancellation.
// A canceled inbound call must still be able to persist turn_end.
func finalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), turnFinalizeTimeout)
}
