package main

import (
	"context"
)

const cancelledWindow = 256

func (p *plugin) registerCall(callID string, cancel context.CancelFunc) {
	if callID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelled[callID] {
		cancel()
		return
	}
	p.active[callID] = cancel
}

func (p *plugin) cancelCall(callID string) bool {
	if callID == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel, ok := p.active[callID]
	if !ok {
		p.rememberCancelLocked(callID)
		return false
	}
	cancel()
	delete(p.active, callID)
	return true
}

func (p *plugin) rememberCancelLocked(callID string) {
	if p.cancelled[callID] {
		return
	}
	p.cancelled[callID] = true
	p.cancelledOrder = append(p.cancelledOrder, callID)
	for len(p.cancelledOrder) > cancelledWindow {
		oldest := p.cancelledOrder[0]
		p.cancelledOrder = p.cancelledOrder[1:]
		delete(p.cancelled, oldest)
	}
}

func (p *plugin) finishCall(callID string) {
	if callID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, callID)
	delete(p.cancelled, callID)
}
