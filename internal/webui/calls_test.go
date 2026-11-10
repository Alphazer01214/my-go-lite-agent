package webui

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func eventFrame(seq int, payload string) *protocol.Frame {
	return &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         "ui-call-test",
		Type:       protocol.FrameEvent,
		Capability: "loop",
		Method:     "chunk",
		Payload:    json.RawMessage(`{"n":"` + payload + `"}`),
	}
}

func finalFrame(payload string) *protocol.Frame {
	return &protocol.Frame{
		Version:    protocol.FrameVersion,
		ID:         "ui-call-test",
		Type:       protocol.FrameResponse,
		Capability: "loop",
		Method:     "turn",
		Payload:    json.RawMessage(payload),
	}
}

func TestCallHubReplaysBufferedEvents(t *testing.T) {
	hub := newCallHub(nil)
	record := hub.begin("c1", "loop", "turn", json.RawMessage(`{"session_id":"s1"}`), nil)
	for i := 1; i <= 3; i++ {
		record.appendFrame(eventFrame(i, string(rune('a'+i-1))))
	}

	snapshot, live, cancel := record.attach(1)
	defer cancel()
	if snapshot.Status != "running" || snapshot.NextSeq != 3 || snapshot.Truncated {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if len(snapshot.Replay) != 2 || snapshot.Replay[0].Seq != 2 || snapshot.Replay[1].Seq != 3 {
		t.Fatalf("unexpected replay: %+v", snapshot.Replay)
	}

	record.appendFrame(eventFrame(4, "d"))
	select {
	case event := <-live:
		if event.Seq != 4 || event.Final {
			t.Fatalf("unexpected live event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("live event was not delivered")
	}

	hub.finish(record, finalFrame(`{"ok":true}`), false)
	select {
	case event := <-live:
		if !event.Final || event.Frame == nil || event.Frame.ErrorCode != "" {
			t.Fatalf("unexpected final event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("final event was not delivered")
	}

	snapshot, live, cancel = record.attach(0)
	cancel()
	if snapshot.Status != "finished" || snapshot.Final == nil {
		t.Fatalf("unexpected finished snapshot: %+v", snapshot)
	}
	if live != nil {
		t.Fatal("finished call must not expose a live channel")
	}
	if len(snapshot.Replay) != 4 || snapshot.FirstSeq != 1 || snapshot.Truncated {
		t.Fatalf("unexpected finished replay: %+v", snapshot)
	}

	calls := hub.list()
	if len(calls) != 1 || calls[0].CallID != "c1" || calls[0].EventCount != 4 {
		t.Fatalf("unexpected list: %+v", calls)
	}
}

func TestCallHubDropsCallsWithoutEvents(t *testing.T) {
	hub := newCallHub(nil)
	record := hub.begin("c1", "session", "info", json.RawMessage(`{}`), nil)
	hub.finish(record, finalFrame(`{}`), false)

	if calls := hub.list(); len(calls) != 0 {
		t.Fatalf("eventless call must not stay attachable: %+v", calls)
	}
	if _, ok := hub.lookup("c1"); ok {
		t.Fatal("eventless call must be removed")
	}
}

func TestCallHubTruncatesOldEvents(t *testing.T) {
	hub := newCallHub(nil)
	hub.maxEvents = 2
	hub.maxBytes = 1 << 20
	record := hub.begin("c1", "loop", "turn", json.RawMessage(`{}`), nil)
	for i := 1; i <= 4; i++ {
		record.appendFrame(eventFrame(i, strings.Repeat("x", i)))
	}

	snapshot, _, cancel := record.attach(0)
	cancel()
	if !snapshot.Truncated {
		t.Fatal("snapshot must report truncation")
	}
	if snapshot.FirstSeq != 3 || len(snapshot.Replay) != 2 {
		t.Fatalf("unexpected truncated replay: first=%d len=%d", snapshot.FirstSeq, len(snapshot.Replay))
	}
}

func TestCallHubLifecycleCallbacks(t *testing.T) {
	started := make(chan string, 1)
	finished := make(chan string, 1)
	hub := newCallHub(func(record *callRecord) { finished <- record.id })
	record := hub.begin("c1", "loop", "turn", json.RawMessage(`{}`), func(record *callRecord) {
		started <- record.id
	})
	record.appendFrame(eventFrame(1, "a"))
	record.appendFrame(eventFrame(2, "b"))
	select {
	case id := <-started:
		if id != "c1" {
			t.Fatalf("unexpected start id: %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("start callback was not invoked")
	}
	select {
	case <-started:
		t.Fatal("start callback must fire once")
	default:
	}

	hub.finish(record, finalFrame(`{}`), false)
	select {
	case id := <-finished:
		if id != "c1" {
			t.Fatalf("unexpected finish id: %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("finish callback was not invoked")
	}
}

func TestReplayAfterSkipsNonReplayableAndReportsGap(t *testing.T) {
	server := NewServer(nil, nil)
	server.enqueueUI(&uiEnvelope{Version: protocol.FrameVersion, ID: "1", Type: "call"})
	server.enqueueUI(&uiEnvelope{Version: protocol.FrameVersion, ID: "2", Type: "ui"})
	server.enqueueUI(&uiEnvelope{Version: protocol.FrameVersion, ID: "3", Type: "diagnostic"})

	events, gap := server.replayAfter(0)
	if gap {
		t.Fatal("after=0 must not be treated as a gap")
	}
	if len(events) != 2 || events[0].RSeq != 1 || events[1].RSeq != 2 {
		t.Fatalf("unexpected replay: %+v", events)
	}

	server.replayMu.Lock()
	server.replay = []*uiEnvelope{{Version: protocol.FrameVersion, ID: "4", Type: "call", RSeq: 3}}
	server.replayMu.Unlock()
	if _, gap := server.replayAfter(1); !gap {
		t.Fatal("missing replay seq must report a gap")
	}
}

type callBackend struct {
	events  []*protocol.Frame
	final   *protocol.Frame
	release chan struct{}
	done    chan struct{}
}

func (b *callBackend) CallWithEvents(
	_ context.Context,
	_, _, _ string,
	_ json.RawMessage,
	onEvent func(*protocol.Frame),
) (*protocol.Frame, error) {
	for _, event := range b.events {
		onEvent(event)
	}
	if b.release != nil {
		<-b.release
	}
	if b.done != nil {
		close(b.done)
	}
	return b.final, nil
}

func browserCallRequest(t *testing.T, id string) *http.Request {
	t.Helper()
	body := `{"version":1,"id":"` + id + `","type":"req","capability":"loop","method":"turn","payload":{"session_id":"s1"}}`
	request := httptest.NewRequest(http.MethodPost, "/bridge/call", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestServeBrowserCallBuffersForAttach(t *testing.T) {
	backend := &callBackend{
		events: []*protocol.Frame{eventFrame(1, "a"), eventFrame(2, "b")},
		final:  finalFrame(`{"ok":true}`),
	}
	server := NewServer(nil, backend)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, browserCallRequest(t, "ui-call-test"))

	lines := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected two events and a response, got %d lines: %s", len(lines), recorder.Body.String())
	}

	attach := httptest.NewRecorder()
	server.Handler().ServeHTTP(attach, httptest.NewRequest(http.MethodGet, "/bridge/calls/ui-call-test", nil))
	attachLines := strings.Split(strings.TrimSpace(attach.Body.String()), "\n")
	if len(attachLines) != 5 {
		t.Fatalf("expected replay, two events, replay_end and result, got %d lines: %s", len(attachLines), attach.Body.String())
	}
	var meta callAttachMeta
	if err := json.Unmarshal([]byte(attachLines[0]), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Type != "replay" || meta.Status != "finished" || meta.FirstSeq != 1 || meta.NextSeq != 2 {
		t.Fatalf("unexpected attach meta: %+v", meta)
	}
	var replayEnd struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(attachLines[3]), &replayEnd); err != nil {
		t.Fatal(err)
	}
	if replayEnd.Type != "replay_end" {
		t.Fatalf("unexpected replay end: %s", attachLines[3])
	}
	var result callAttachResult
	if err := json.Unmarshal([]byte(attachLines[4]), &result); err != nil {
		t.Fatal(err)
	}
	if result.Type != "result" || result.Status != "finished" || result.Response == nil {
		t.Fatalf("unexpected attach result: %+v", result)
	}
}

func TestServeBrowserCallDropsEventlessCall(t *testing.T) {
	server := NewServer(nil, &callBackend{final: finalFrame(`{"ok":true}`)})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, browserCallRequest(t, "ui-call-test"))
	if calls := server.callHub.list(); len(calls) != 0 {
		t.Fatalf("eventless call must not be attachable: %+v", calls)
	}
}

func TestServeBrowserCallKeepsRunningAfterDisconnect(t *testing.T) {
	done := make(chan struct{})
	release := make(chan struct{})
	backend := &callBackend{
		events:  []*protocol.Frame{eventFrame(1, "a")},
		final:   finalFrame(`{"ok":true}`),
		release: release,
		done:    done,
	}
	server := NewServer(nil, backend)

	request := browserCallRequest(t, "ui-call-test")
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)
	server.Handler().ServeHTTP(httptest.NewRecorder(), request)

	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backend call did not finish")
	}
	deadline := time.Now().Add(time.Second)
	for {
		record, ok := server.callHub.lookup("ui-call-test")
		if ok {
			snapshot, live, cancelAttach := record.attach(0)
			cancelAttach()
			if snapshot.Status == "finished" && live == nil {
				if len(snapshot.Replay) != 1 || snapshot.Final == nil {
					t.Fatalf("unexpected recovered call: %+v", snapshot)
				}
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnected call was not retained")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestServeUIReplaysCallLifecycle(t *testing.T) {
	server := NewServer(nil, nil)
	server.enqueueUI(&uiEnvelope{Version: protocol.FrameVersion, ID: "1", Type: "call"})
	server.enqueueUI(&uiEnvelope{Version: protocol.FrameVersion, ID: "2", Type: "call"})

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/bridge/ui?after=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	var kinds []string
	for scanner.Scan() {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, envelope.Type)
		if len(kinds) == 2 {
			break
		}
	}
	cancel()
	if len(kinds) != 2 || kinds[0] != "hello" || kinds[1] != "call" {
		t.Fatalf("unexpected UI stream: %v", kinds)
	}
}
