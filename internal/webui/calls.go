package webui

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

const (
	// A finished call stays attachable for a while so a reload can collect its
	// result without racing the browser response.
	defaultCallTTL = 10 * time.Minute
	// Buffers are bounded per call: long turns keep only the newest events and
	// report truncation to late attachers.
	defaultCallMaxEvents = 8192
	defaultCallMaxBytes  = 1 << 20
	// One attached page is a slow consumer only for itself; the call keeps
	// running and the page can reattach from its last seq.
	defaultCallSubscriberBuffer = 256
)

type callEvent struct {
	Seq    uint64
	Status string
	Frame  *protocol.Frame
	Final  bool
}

type callSubscriber struct {
	ch chan callEvent
}

// callRecord is the host-side memory of one browser-initiated call. It is
// independent from the HTTP response so a reload can reattach.
type callRecord struct {
	id         string
	capability string
	method     string
	request    json.RawMessage
	startedAt  time.Time
	maxEvents  int
	maxBytes   int

	mu         sync.Mutex
	status     string // running | finished | failed
	events     []callEvent
	bytes      int
	hasEvents  bool
	nextSeq    uint64
	firstSeq   uint64
	truncated  bool
	final      *protocol.Frame
	finishedAt time.Time
	subs       map[*callSubscriber]struct{}
	onStart    func(*callRecord)
}

type callSummary struct {
	CallID     string          `json:"call_id"`
	Capability string          `json:"capability"`
	Method     string          `json:"method"`
	Request    json.RawMessage `json:"request,omitempty"`
	Status     string          `json:"status"`
	StartedAt  int64           `json:"started_at"`
	FinishedAt int64           `json:"finished_at,omitempty"`
	EventCount int             `json:"event_count"`
	FirstSeq   uint64          `json:"first_seq"`
	NextSeq    uint64          `json:"next_seq"`
	Truncated  bool            `json:"truncated"`
}

type callAttachSnapshot struct {
	Capability string
	Method     string
	Request    json.RawMessage
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	FirstSeq   uint64
	NextSeq    uint64
	Truncated  bool
	Final      *protocol.Frame
	Replay     []callEvent
}

// callHub owns every browser call known to this Host process. Calls without
// events are removed as soon as they finish: unary capability calls have
// nothing worth attaching to.
type callHub struct {
	mu        sync.Mutex
	calls     map[string]*callRecord
	ttl       time.Duration
	maxEvents int
	maxBytes  int
	onFinish  func(*callRecord)
}

func newCallHub(onFinish func(*callRecord)) *callHub {
	return &callHub{
		calls:     make(map[string]*callRecord),
		ttl:       defaultCallTTL,
		maxEvents: defaultCallMaxEvents,
		maxBytes:  defaultCallMaxBytes,
		onFinish:  onFinish,
	}
}

func (h *callHub) begin(
	id, capability, method string,
	request json.RawMessage,
	onStart func(*callRecord),
) *callRecord {
	h.mu.Lock()
	h.cleanupLocked(time.Now())
	rec := &callRecord{
		id:         id,
		capability: capability,
		method:     method,
		request:    append(json.RawMessage(nil), request...),
		startedAt:  time.Now(),
		maxEvents:  h.maxEvents,
		maxBytes:   h.maxBytes,
		status:     "running",
		subs:       make(map[*callSubscriber]struct{}),
		onStart:    onStart,
	}
	h.calls[id] = rec
	h.mu.Unlock()
	return rec
}

func (h *callHub) lookup(id string) (*callRecord, bool) {
	h.mu.Lock()
	rec, ok := h.calls[id]
	h.mu.Unlock()
	return rec, ok
}

func (h *callHub) list() []callSummary {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked(time.Now())
	out := make([]callSummary, 0, len(h.calls))
	for _, rec := range h.calls {
		out = append(out, rec.summary())
	}
	return out
}

func (h *callHub) finish(rec *callRecord, frame *protocol.Frame, failed bool) {
	broadcast, tracked := rec.finish(frame, failed)
	if !tracked {
		h.mu.Lock()
		delete(h.calls, rec.id)
		h.mu.Unlock()
		return
	}
	if broadcast && h.onFinish != nil {
		h.onFinish(rec)
	}
}

func (h *callHub) cleanupLocked(now time.Time) {
	for id, rec := range h.calls {
		if rec.expired(now, h.ttl) {
			delete(h.calls, id)
		}
	}
}

func (r *callRecord) summary() callSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.summaryLocked()
}

func (r *callRecord) summaryLocked() callSummary {
	out := callSummary{
		CallID:     r.id,
		Capability: r.capability,
		Method:     r.method,
		Request:    append(json.RawMessage(nil), r.request...),
		Status:     r.status,
		StartedAt:  r.startedAt.UnixMilli(),
		EventCount: len(r.events),
		FirstSeq:   r.firstSeq,
		NextSeq:    r.nextSeq,
		Truncated:  r.truncated,
	}
	if !r.finishedAt.IsZero() {
		out.FinishedAt = r.finishedAt.UnixMilli()
	}
	return out
}

func (r *callRecord) expired(now time.Time, ttl time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status != "running" && !r.finishedAt.IsZero() && now.Sub(r.finishedAt) > ttl
}

// appendFrame stores one browser-shaped event. The first event also announces
// the call on the UI bus so sidebars can show a running marker.
func (r *callRecord) appendFrame(frame *protocol.Frame) {
	r.mu.Lock()
	if r.status != "running" {
		r.mu.Unlock()
		return
	}
	r.nextSeq++
	event := callEvent{Seq: r.nextSeq, Frame: cloneFrame(frame)}
	r.events = append(r.events, event)
	r.bytes += frameBytes(frame)
	if r.firstSeq == 0 {
		r.firstSeq = event.Seq
	}
	first := !r.hasEvents
	r.hasEvents = true
	r.trimLocked()
	r.fanoutLocked(event)
	onStart := r.onStart
	if first {
		r.onStart = nil
	}
	r.mu.Unlock()

	if first && onStart != nil {
		onStart(r)
	}
}

func (r *callRecord) trimLocked() {
	for len(r.events) > 0 && (len(r.events) > r.maxEvents || r.bytes > r.maxBytes) {
		r.bytes -= frameBytes(r.events[0].Frame)
		r.events = r.events[1:]
		r.truncated = true
	}
	if len(r.events) > 0 {
		r.firstSeq = r.events[0].Seq
	}
}

// finish closes the call and returns whether the call had events (which keeps
// it attachable) and whether the UI bus should announce the terminal state.
func (r *callRecord) finish(frame *protocol.Frame, failed bool) (broadcast, tracked bool) {
	r.mu.Lock()
	if r.status != "running" {
		r.mu.Unlock()
		return false, true
	}
	if failed {
		r.status = "failed"
	} else {
		r.status = "finished"
	}
	r.final = cloneFrame(frame)
	r.finishedAt = time.Now()
	broadcast = r.hasEvents
	tracked = r.hasEvents
	r.fanoutLocked(callEvent{Final: true, Status: r.status, Frame: cloneFrame(frame)})
	r.mu.Unlock()
	return broadcast, tracked
}

func (r *callRecord) fanoutLocked(event callEvent) {
	for sub := range r.subs {
		select {
		case sub.ch <- event:
		default:
			delete(r.subs, sub)
			close(sub.ch)
		}
	}
	// A final event retires every subscriber; the handler will read it and the
	// closed channel stops the loop.
	if event.Final {
		for sub := range r.subs {
			delete(r.subs, sub)
			close(sub.ch)
		}
	}
}

// attach returns the current snapshot, buffered events after the requested
// seq, and a live channel while the call is running. after=0 replays every
// retained event.
func (r *callRecord) attach(after uint64) (callAttachSnapshot, <-chan callEvent, func()) {
	r.mu.Lock()
	snap := callAttachSnapshot{
		Capability: r.capability,
		Method:     r.method,
		Request:    append(json.RawMessage(nil), r.request...),
		Status:     r.status,
		StartedAt:  r.startedAt,
		FinishedAt: r.finishedAt,
		FirstSeq:   r.firstSeq,
		NextSeq:    r.nextSeq,
		Truncated:  r.truncated,
		Final:      cloneFrame(r.final),
	}
	for _, event := range r.events {
		if after == 0 || event.Seq > after {
			snap.Replay = append(snap.Replay, event)
		}
	}
	if r.status != "running" {
		r.mu.Unlock()
		return snap, nil, func() {}
	}
	sub := &callSubscriber{ch: make(chan callEvent, defaultCallSubscriberBuffer)}
	r.subs[sub] = struct{}{}
	cancel := func() {
		r.mu.Lock()
		if _, ok := r.subs[sub]; ok {
			delete(r.subs, sub)
			close(sub.ch)
		}
		r.mu.Unlock()
	}
	r.mu.Unlock()
	return snap, sub.ch, cancel
}

func cloneFrame(frame *protocol.Frame) *protocol.Frame {
	if frame == nil {
		return nil
	}
	out := *frame
	out.Payload = append(json.RawMessage(nil), frame.Payload...)
	return &out
}

func frameBytes(frame *protocol.Frame) int {
	if frame == nil {
		return 0
	}
	return len(frame.Payload) + len(frame.ErrorMsg) + 128
}
