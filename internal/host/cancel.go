package host

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	cancelReasonCancelled = "cancelled"
	cancelReasonDeadline  = "deadline_exceeded"
)

func cancelReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return cancelReasonDeadline
	}
	return cancelReasonCancelled
}

func cancelReasonCode(reason string) string {
	if reason == cancelReasonDeadline {
		return CodeCallDeadlineExceeded
	}
	return CodeCallCancelled
}

func (h *Host) cancelGraceDuration() time.Duration {
	if h.cancelGrace > 0 {
		return h.cancelGrace
	}
	return DefaultCancelGrace
}

// cancelPending forwards a cancellation and arms the Host-owned terminal timer.
func (h *Host) cancelPending(frameID, reason string) error {
	h.mu.Lock()
	key := frameID
	wt := h.pending[key]
	if wt == nil {
		for candidateKey, candidate := range h.pending {
			if candidate.frameID == frameID {
				key, wt = candidateKey, candidate
				break
			}
		}
	}
	if wt == nil {
		h.mu.Unlock()
		return fmt.Errorf("call %s is not pending: %w", frameID, ErrTimeout)
	}
	if wt.targetGeneration != 0 {
		if p := h.plugins[wt.target]; p == nil || p.gen != wt.targetGeneration {
			h.mu.Unlock()
			return fmt.Errorf("call %s target generation changed: %w", frameID, ErrContextBinding)
		}
	}
	if wt.cancelReason == "" {
		wt.cancelReason = reason
		wt.cancelTimer = time.AfterFunc(h.cancelGraceDuration(), func() {
			h.cancelTimeout(key, wt.targetGeneration, reason)
		})
	}
	target := wt.target
	from := wt.callerPlugin
	contextID := wt.contextID
	capability := wt.capability
	method := wt.method
	h.mu.Unlock()

	err := h.write(target, &Frame{
		Version:    FrameVersion,
		ID:         key,
		Type:       FrameCancel,
		From:       from,
		ContextID:  contextID,
		Capability: capability,
		Method:     method,
		Reason:     reason,
	})
	if err != nil {
		h.mu.Lock()
		if current := h.pending[key]; current == wt {
			delete(h.pending, key)
		}
		h.mu.Unlock()
		h.failWait(wt, cancelReasonCode(reason), err.Error())
		return err
	}
	return nil
}

func (h *Host) handleCancel(from string, frame *Frame) {
	h.mu.Lock()
	var key string
	var wt *wait
	for candidateKey, candidate := range h.pending {
		if candidate.frameID == frame.ID && candidate.caller == from {
			key, wt = candidateKey, candidate
			break
		}
	}
	h.mu.Unlock()
	if wt == nil {
		return
	}
	reason := frame.Reason
	if reason != cancelReasonDeadline {
		reason = cancelReasonCancelled
	}
	_ = h.cancelPending(key, reason)
}

func (h *Host) cancelTimeout(key string, generation int, reason string) {
	h.mu.Lock()
	wt := h.pending[key]
	if wt == nil || wt.targetGeneration != generation || wt.cancelReason != reason {
		h.mu.Unlock()
		return
	}
	delete(h.pending, key)
	h.mu.Unlock()
	h.failWait(wt, cancelReasonCode(reason), "call cancellation grace exceeded")
}

func stopWaitTimer(wt *wait) {
	if wt != nil && wt.cancelTimer != nil {
		wt.cancelTimer.Stop()
		wt.cancelTimer = nil
	}
}

func (h *Host) cancelPendingWhere(match func(*wait) bool, reason string) int {
	h.mu.Lock()
	keys := make([]string, 0, len(h.pending))
	for key, wt := range h.pending {
		if match(wt) {
			keys = append(keys, key)
		}
	}
	h.mu.Unlock()
	for _, key := range keys {
		_ = h.cancelPending(key, reason)
	}
	return len(keys)
}
