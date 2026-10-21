package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// FactTypeDecision is the session fact type of one adjudication entry. The
// type is registered in the session vocabulary (plugins/session/types.go).
const FactTypeDecision = "sandbox_decision"

// auditTimeout bounds one audit write. The write must not hold up the call it
// describes for longer than this.
const auditTimeout = 3 * time.Second

// auditRingLimit bounds the in-memory fallback buffer.
const auditRingLimit = 256

// auditMeta is the meta block of one audit entry. It never carries the
// argument text, only a digest of it.
type auditMeta struct {
	CallID           string `json:"call_id"`
	Tool             string `json:"tool"`
	TargetCapability string `json:"target_capability"`
	TargetMethod     string `json:"target_method"`
	Decision         string `json:"decision"`
	RuleID           string `json:"rule_id"`
	Reason           string `json:"reason"`
	Risk             int    `json:"risk"`
	ArgumentsDigest  string `json:"arguments_digest"`
	SessionID        string `json:"session_id"`
	TurnID           string `json:"turn_id"`
	StepID           string `json:"step_id"`
	ConfirmToken     string `json:"confirm_token"`
	TS               int64  `json:"ts"`
}

// auditLog is the bounded in-memory fallback. It is used when the session
// plugin is absent or the write fails: the entry is then only good for
// troubleshooting in the running process.
type auditLog struct {
	mu    sync.Mutex
	ring  []auditMeta
	limit int
}

// append adds one entry and drops the oldest ones over the limit.
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

// entries returns a copy of the buffered entries.
func (a *auditLog) entries() []auditMeta {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]auditMeta, len(a.ring))
	copy(out, a.ring)
	return out
}

// writeAudit records one adjudication.
//
// The entry goes to the session fact log, so it shares the timeline of the
// call it describes. When the session plugin is absent or the write fails,
// the entry falls back to the in-memory ring buffer.
//
// The write uses a context that outlives the call: a cancelled turn still
// gets its decision recorded.
func (p *plugin) writeAudit(ctx context.Context, d decision, call callContext, arguments json.RawMessage) {
	// A call that was cancelled before the decision leaves no trace: there was
	// no decision to record.
	if ctx.Err() != nil {
		return
	}
	entry := auditMeta{
		CallID:           call.CallID,
		Tool:             call.Tool,
		TargetCapability: call.Capability,
		TargetMethod:     call.Method,
		Decision:         d.Action,
		RuleID:           d.RuleID,
		Reason:           d.Reason,
		Risk:             d.Risk,
		ArgumentsDigest:  Digest(arguments),
		SessionID:        call.SessionID,
		TurnID:           call.TurnID,
		StepID:           call.StepID,
		ConfirmToken:     "",
		TS:               time.Now().UnixMilli(),
	}
	if call.SessionID == "" {
		p.audit.append(entry)
		return
	}
	payload := mustJSON(map[string]any{
		"session_id": call.SessionID,
		"facts": []map[string]any{{
			"type":    FactTypeDecision,
			"role":    "host",
			"content": "",
			"meta":    entry,
		}},
	})
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditTimeout)
	defer cancel()
	if _, err := p.sdk.Call(writeCtx, "session", "append", payload); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: audit: %v\n", err)
		p.audit.append(entry)
	}
}
