package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type stubCallBackend struct {
	calls   int
	from    string
	method  string
	payload json.RawMessage
	onEvent func(*protocol.Frame)
	events  []*protocol.Frame
	result  *protocol.Frame
	err     error
}

func (s *stubCallBackend) CallWithEvents(
	from string,
	capability string,
	method string,
	payload json.RawMessage,
	onEvent func(*protocol.Frame),
) (*protocol.Frame, error) {
	s.calls++
	s.from = from
	s.method = method
	s.payload = append(json.RawMessage(nil), payload...)
	s.onEvent = onEvent
	for _, event := range s.events {
		onEvent(event)
	}
	return s.result, s.err
}

func decodeNDJSON(t *testing.T, body string) []*protocol.Frame {
	t.Helper()
	var frames []*protocol.Frame
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame protocol.Frame
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode NDJSON line %q: %v", line, err)
		}
		frames = append(frames, &frame)
	}
	return frames
}

func browserCallRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPost, "/bridge/call", strings.NewReader(body))
}

func TestBrowserCallStreamsEventsThenResponse(t *testing.T) {
	backend := &stubCallBackend{
		events: []*protocol.Frame{
			{
				Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameEvent,
				From: "session", Capability: "session", Method: "progress",
				Payload: json.RawMessage(`{"step":1}`),
			},
			{
				Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameEvent,
				From: "session", Capability: "session", Method: "progress",
				Payload: json.RawMessage(`{"step":2}`),
			},
		},
		result: &protocol.Frame{
			Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameResponse,
			From: "session", Capability: "session", Method: "query",
			Payload: json.RawMessage(`{"ok":true}`),
		},
	}
	handler := NewServer(nil, backend).Handler()

	request := browserCallRequest(t, `{
		"version": 1,
		"id": "ui-call-7",
		"type": "req",
		"capability": "session",
		"method": "query",
		"payload": {"session_id":"s1"}
	}`)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/x-ndjson" {
		t.Fatalf("content type = %q", got)
	}
	if backend.calls != 1 || backend.from != "webui" || backend.method != "query" {
		t.Fatalf("backend call = %#v", backend)
	}
	if string(backend.payload) != `{"session_id":"s1"}` {
		t.Fatalf("payload = %s", backend.payload)
	}

	frames := decodeNDJSON(t, recorder.Body.String())
	if len(frames) != 3 {
		t.Fatalf("frames = %#v", frames)
	}
	for _, frame := range frames {
		if frame.ID != "ui-call-7" {
			t.Fatalf("frame id = %q", frame.ID)
		}
	}
	if frames[0].Type != protocol.FrameEvent || frames[1].Type != protocol.FrameEvent || frames[2].Type != protocol.FrameResponse {
		t.Fatalf("frame types = %#v", frames)
	}
	if frames[0].Method != "progress" || frames[1].Method != "progress" || frames[2].Method != "query" {
		t.Fatalf("frame methods = %#v", frames)
	}
}

func TestBrowserCallRejectsInvalidBodyBeforeBackend(t *testing.T) {
	backend := &stubCallBackend{}
	handler := NewServer(nil, backend).Handler()

	for _, body := range []string{
		`{`,
		`{"version":1,"id":"ui-call-1","type":"req","capability":"llm","method":"complete","from":"host","payload":{}}`,
		`{"version":1,"id":"ui-call-1","type":"req","capability":"llm","method":"complete","payload":{}} {}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, browserCallRequest(t, body))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
	if backend.calls != 0 {
		t.Fatalf("backend calls = %d", backend.calls)
	}
}

func TestBrowserCallReturnsStableErrorResponses(t *testing.T) {
	for _, test := range []struct {
		name      string
		body      string
		backend   *stubCallBackend
		errorCode string
	}{
		{
			name:      "host capability",
			body:      `{"version":1,"id":"ui-call-1","type":"req","capability":"host","method":"plugins","payload":{}}`,
			backend:   &stubCallBackend{},
			errorCode: protocol.CodeMethodNotFound,
		},
		{
			name:      "backend error",
			body:      `{"version":1,"id":"ui-call-2","type":"req","capability":"ghost","method":"run","payload":{}}`,
			backend:   &stubCallBackend{err: errors.Join(host.ErrMethodNotFound, errors.New("no owner"))},
			errorCode: protocol.CodeMethodNotFound,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := NewServer(nil, test.backend).Handler()
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, browserCallRequest(t, test.body))

			frames := decodeNDJSON(t, recorder.Body.String())
			if len(frames) != 1 || frames[0].Type != protocol.FrameResponse {
				t.Fatalf("frames = %#v", frames)
			}
			if recorder.Header().Get("Content-Type") != "application/x-ndjson" {
				t.Fatalf("content type = %q", recorder.Header().Get("Content-Type"))
			}
			if frames[0].ID == "" || frames[0].ErrorCode != test.errorCode {
				t.Fatalf("response = %#v", frames[0])
			}
			if test.name == "host capability" && test.backend.calls != 0 {
				t.Fatalf("host call reached backend")
			}
		})
	}
}

func TestBrowserCallReturnsOneResponseWithoutEvents(t *testing.T) {
	backend := &stubCallBackend{result: &protocol.Frame{
		Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameResponse,
		From: "demo", Capability: "demo", Method: "run",
	}}
	handler := NewServer(nil, backend).Handler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, browserCallRequest(t,
		`{"version":1,"id":"ui-call-1","type":"req","capability":"demo","method":"run","payload":{}}`))

	frames := decodeNDJSON(t, recorder.Body.String())
	if len(frames) != 1 || frames[0].Type != protocol.FrameResponse || frames[0].ID != "ui-call-1" {
		t.Fatalf("frames = %#v", frames)
	}
}
