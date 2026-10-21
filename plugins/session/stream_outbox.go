package main

import (
	"encoding/json"
	"time"
)

const streamFlushInterval = 33 * time.Millisecond

type streamOutDelta struct {
	streamDeltaUI
	Coalesced bool `json:"coalesced,omitempty"`
}

// startStreamOutbox starts one publisher goroutine. Tests call
// flushStreamOutbox directly, so the goroutine is owned by main.
func (p *plugin) startStreamOutbox() {
	p.streamOnce.Do(func() {
		go p.runStreamOutbox()
	})
}

func (p *plugin) runStreamOutbox() {
	ticker := time.NewTicker(streamFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.streamStop:
			return
		case <-ticker.C:
			p.flushStreamOutbox()
		}
	}
}

// enqueueStream coalesces deltas for one session. Content and reasoning are
// concatenated, so a skipped frame never loses text. ClearStream drops pending
// output when the final assistant message lands.
func (p *plugin) enqueueStream(delta streamDeltaUI) {
	p.streamOutMu.Lock()
	pending := p.streamOut[delta.SessionID]
	switch {
	case pending == nil || delta.Reset || pending.TurnID != delta.TurnID || pending.StepID != delta.StepID:
		pending = &streamOutDelta{streamDeltaUI: delta}
	default:
		pending.ContentDelta += delta.ContentDelta
		pending.ReasoningDelta += delta.ReasoningDelta
		pending.UpdateID = delta.UpdateID
		pending.Status = delta.Status
		pending.Coalesced = true
	}
	p.streamOut[delta.SessionID] = pending
	p.streamOutMu.Unlock()
}

func (p *plugin) flushStreamOutbox() {
	p.streamOutMu.Lock()
	pending := p.streamOut
	p.streamOut = make(map[string]*streamOutDelta)
	p.streamOutMu.Unlock()
	if len(pending) == 0 {
		return
	}

	for _, delta := range pending {
		p.mu.RLock()
		ui := p.ui
		open := p.isOpenLocked(delta.SessionID)
		p.mu.RUnlock()
		if ui == nil || !open {
			continue
		}
		payload, err := json.Marshal(delta)
		if err != nil {
			continue
		}
		// SendCommand has its own bounded context. A slow Host only delays this
		// publisher; stream.update handlers continue to coalesce into pending.
		_ = ui.SendCommand("chat", "stream", payload)
	}
}

func (p *plugin) dropStream(sessionID, turnID, stepID string) {
	p.streamOutMu.Lock()
	pending := p.streamOut[sessionID]
	if pending != nil &&
		(turnID == "" || pending.TurnID == turnID) &&
		(stepID == "" || pending.StepID == stepID) {
		delete(p.streamOut, sessionID)
	}
	p.streamOutMu.Unlock()
}
