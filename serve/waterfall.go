package serve

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/protocol"
)

// InterceptorCap is the Capability Interceptor Plugins provide.
const InterceptorCap = "interceptor"

// Waterfall actions returned by the built-in chain and external Interceptors.
const (
	ActionAllow   = "allow"
	ActionRewrite = "rewrite"
	ActionReject  = "reject"
)

// WaterfallRequest is one call presented to Interceptors.
type WaterfallRequest struct {
	From    string          `json:"from"`
	Cap     string          `json:"cap"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// WaterfallDecision is the outcome of one Waterfall step.
type WaterfallDecision struct {
	Action  string          `json:"action"`
	Code    string          `json:"code,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

// AuditEntry is one built-in audit record (mandatory chain).
type AuditEntry struct {
	From   string `json:"from"`
	Cap    string `json:"cap"`
	Method string `json:"method"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// Audit returns a copy of the built-in audit trail.
func (s *Server) Audit() []AuditEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]AuditEntry, len(s.audit))
	copy(out, s.audit)
	return out
}

func (s *Server) recordAudit(from, capName, method, action, reason string) {
	e := AuditEntry{From: from, Cap: capName, Method: method, Action: action, Reason: reason}
	s.mu.Lock()
	s.audit = append(s.audit, e)
	s.mu.Unlock()
}

// runWaterfall executes the dual chain: built-in cancel → external Interceptor → built-in audit.
// The mandatory chain cannot be removed by Interceptors; a down Interceptor is skipped (fail-open).
func (s *Server) runWaterfall(from string, f *protocol.Frame) WaterfallDecision {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		dec := WaterfallDecision{Action: ActionReject, Code: "host_closed", Reason: "host_closed"}
		s.recordAudit(from, f.Cap, f.Method, dec.Action, dec.Reason)
		return dec
	}

	dec := s.callInterceptor(from, f)
	s.recordAudit(from, f.Cap, f.Method, dec.Action, dec.Reason)
	return dec
}

func (s *Server) callInterceptor(from string, f *protocol.Frame) WaterfallDecision {
	s.mu.Lock()
	owner, ok := s.provides[InterceptorCap]
	s.mu.Unlock()
	if !ok {
		return WaterfallDecision{Action: ActionAllow, Reason: "no_interceptor"}
	}

	body, err := json.Marshal(WaterfallRequest{
		From:    from,
		Cap:     f.Cap,
		Method:  f.Method,
		Payload: f.Payload,
	})
	if err != nil {
		return WaterfallDecision{Action: ActionAllow, Reason: "interceptor_encode_error"}
	}
	res, err := s.Call(owner, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     InterceptorCap,
		Method:  "before",
		Payload: body,
	})
	if err != nil {
		// Fail-open: a crashed Interceptor must not take down the mandatory chain or star routing.
		return WaterfallDecision{Action: ActionAllow, Reason: "interceptor_down"}
	}
	if res.Error != nil {
		// plugin_down after on-demand restart still means the Interceptor is unavailable.
		if res.Error.Code == "plugin_down" {
			return WaterfallDecision{Action: ActionAllow, Reason: "interceptor_down"}
		}
		return WaterfallDecision{
			Action: ActionReject,
			Code:   "interceptor_error",
			Reason: "interceptor_error: " + res.Error.Error(),
		}
	}
	var out struct {
		Action  string          `json:"action"`
		Payload json.RawMessage `json:"payload"`
		Reason  string          `json:"reason"`
	}
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &out); err != nil {
			return WaterfallDecision{Action: ActionAllow, Reason: "interceptor_bad_payload"}
		}
	}
	switch out.Action {
	case ActionReject:
		reason := out.Reason
		if reason == "" {
			reason = "interceptor_rejected"
		}
		return WaterfallDecision{Action: ActionReject, Code: "interceptor_rejected", Reason: reason}
	case ActionRewrite:
		// Empty rewrite payload must not wipe the original Call Payload.
		if len(out.Payload) == 0 {
			return WaterfallDecision{Action: ActionAllow, Reason: "rewrite_empty_payload_ignored"}
		}
		return WaterfallDecision{Action: ActionRewrite, Payload: out.Payload, Reason: out.Reason}
	case ActionAllow, "":
		return WaterfallDecision{Action: ActionAllow, Reason: out.Reason}
	default:
		return WaterfallDecision{
			Action: ActionReject,
			Code:   "interceptor_unknown_action",
			Reason: fmt.Sprintf("interceptor_unknown_action %q", out.Action),
		}
	}
}
