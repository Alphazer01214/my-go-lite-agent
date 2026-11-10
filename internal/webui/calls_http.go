package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

type callListResponse struct {
	Version int           `json:"version"`
	Calls   []callSummary `json:"calls"`
}

type callAttachMeta struct {
	Version    int             `json:"version"`
	Type       string          `json:"type"`
	CallID     string          `json:"call_id"`
	Capability string          `json:"capability"`
	Method     string          `json:"method"`
	Request    json.RawMessage `json:"request,omitempty"`
	Status     string          `json:"status"`
	StartedAt  int64           `json:"started_at"`
	FinishedAt int64           `json:"finished_at,omitempty"`
	FirstSeq   uint64          `json:"first_seq"`
	NextSeq    uint64          `json:"next_seq"`
	Truncated  bool            `json:"truncated"`
}

type callAttachEvent struct {
	Version int             `json:"version"`
	Type    string          `json:"type"`
	CallID  string          `json:"call_id"`
	Seq     uint64          `json:"seq"`
	Event   *protocol.Frame `json:"event"`
}

type callAttachResult struct {
	Version  int             `json:"version"`
	Type     string          `json:"type"`
	CallID   string          `json:"call_id"`
	Status   string          `json:"status"`
	Response *protocol.Frame `json:"response,omitempty"`
}

// serveCalls returns the attachable call set. It is the snapshot a reloaded
// page uses before opening any per-call stream.
func (s *Server) serveCalls(w http.ResponseWriter, _ *http.Request) {
	calls := s.callHub.list()
	sort.Slice(calls, func(i, j int) bool {
		a, b := calls[i], calls[j]
		aRunning, bRunning := a.Status == "running", b.Status == "running"
		if aRunning != bRunning {
			return aRunning
		}
		if a.StartedAt != b.StartedAt {
			return a.StartedAt > b.StartedAt
		}
		return a.CallID > b.CallID
	})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(callListResponse{Version: BrowserVersion, Calls: calls}); err != nil {
		http.Error(w, "failed to encode calls", http.StatusInternalServerError)
	}
}

// serveCallAttach streams one call: replay first, then live events. It ends
// with a result envelope so a refreshed page can settle its recovered draft.
func (s *Server) serveCallAttach(w http.ResponseWriter, r *http.Request) {
	callID := strings.TrimSpace(r.PathValue("call"))
	record, ok := s.callHub.lookup(callID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "call_not_found", fmt.Sprintf("call %s is not attached", callID))
		return
	}
	after, _ := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("after")), 10, 64)
	snapshot, live, cancel := record.attach(after)
	defer cancel()

	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := controller.Flush(); err != nil {
		return
	}
	writeEnvelope := func(value any) bool {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			return false
		}
		return controller.Flush() == nil
	}

	meta := callAttachMeta{
		Version:    BrowserVersion,
		Type:       "replay",
		CallID:     callID,
		Capability: snapshot.Capability,
		Method:     snapshot.Method,
		Request:    snapshot.Request,
		Status:     snapshot.Status,
		StartedAt:  snapshot.StartedAt.UnixMilli(),
		FirstSeq:   snapshot.FirstSeq,
		NextSeq:    snapshot.NextSeq,
		Truncated:  snapshot.Truncated,
	}
	if !snapshot.FinishedAt.IsZero() {
		meta.FinishedAt = snapshot.FinishedAt.UnixMilli()
	}
	if !writeEnvelope(meta) {
		return
	}
	for _, event := range snapshot.Replay {
		if !writeEnvelope(callAttachEvent{
			Version: BrowserVersion,
			Type:    "event",
			CallID:  callID,
			Seq:     event.Seq,
			Event:   event.Frame,
		}) {
			return
		}
	}
	// Marks the end of replay. The page can now reconcile its rebuilt draft
	// with already-durable messages before live events continue it.
	if !writeEnvelope(map[string]any{
		"version":   BrowserVersion,
		"type":      "replay_end",
		"call_id":   callID,
		"truncated": snapshot.Truncated,
	}) {
		return
	}
	if live == nil {
		writeEnvelope(callAttachResult{
			Version:  BrowserVersion,
			Type:     "result",
			CallID:   callID,
			Status:   snapshot.Status,
			Response: snapshot.Final,
		})
		return
	}

	heartbeat := time.NewTicker(uiHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case event, ok := <-live:
			if !ok {
				return
			}
			if event.Final {
				writeEnvelope(callAttachResult{
					Version:  BrowserVersion,
					Type:     "result",
					CallID:   callID,
					Status:   event.Status,
					Response: event.Frame,
				})
				return
			}
			if !writeEnvelope(callAttachEvent{
				Version: BrowserVersion,
				Type:    "event",
				CallID:  callID,
				Seq:     event.Seq,
				Event:   event.Frame,
			}) {
				return
			}
		case <-heartbeat.C:
			if !writeEnvelope(map[string]any{
				"version": BrowserVersion,
				"type":    "ping",
				"call_id": callID,
			}) {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
