package main

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	FactTypeDecision = "sandbox_decision"
	auditTimeout     = 3 * time.Second
	auditRingLimit   = 256
)

// auditLog is the bounded in-memory fallback when the session plugin is absent
// or the fact write fails.
type auditLog struct {
	mu    sync.Mutex
	ring  []auditMeta
	limit int
}

func (a *auditLog) append(entry auditMeta) {
	a.mu.Lock()
	defer a.mu.Unlock()
	limit := a.limit
	if limit <= 0 {
		limit = auditRingLimit
	}
	a.ring = append(a.ring, entry)
	if len(a.ring) > limit {
		a.ring = a.ring[len(a.ring)-limit:]
	}
}

// writeAudit records one adjudication on the session timeline. The write uses
// a context that outlives the call: a cancelled turn still gets its decision
// recorded.
func (p *plugin) writeAudit(
	ctx context.Context,
	verdict decision,
	call callContext,
	arguments json.RawMessage,
	confirmToken string,
	expiresAt int64,
) {
	if ctx.Err() != nil {
		return
	}
	entry := auditMeta{
		CallID:           call.CallID,
		Tool:             call.Tool,
		TargetCapability: call.Capability,
		TargetMethod:     call.Method,
		Decision:         verdict.Action,
		RuleID:           verdict.RuleID,
		Reason:           verdict.Reason,
		ArgumentsDigest:  Digest(arguments),
		SessionID:        call.SessionID,
		ConfirmToken:     confirmToken,
		ExpiresAt:        expiresAt,
		TS:               time.Now().UnixMilli(),
	}
	if call.SessionID == "" {
		p.audit.append(entry)
		return
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": call.SessionID,
		"facts": []map[string]any{{
			"type": FactTypeDecision,
			"role": "host",
			"meta": entry,
		}},
	})
	if err != nil {
		p.audit.append(entry)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditTimeout)
	defer cancel()
	if _, err := p.sdk.Call(writeCtx, "session", "append", payload); err != nil {
		logf("audit: %v", err)
		p.audit.append(entry)
	}
}
