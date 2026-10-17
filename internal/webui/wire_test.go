package webui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func TestDecodeBrowserCall(t *testing.T) {
	got, err := decodeBrowserCall(strings.NewReader(`{
		"version": 1,
		"id": "ui-call-1",
		"type": "req",
		"capability": "llm",
		"method": "complete",
		"payload": {"prompt": "hi"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.ID != "ui-call-1" || got.Type != "req" {
		t.Fatalf("request = %#v", got)
	}
	if got.Capability != "llm" || got.Method != "complete" {
		t.Fatalf("request = %#v", got)
	}
	if string(got.Payload) != `{"prompt":"hi"}` {
		t.Fatalf("payload = %s", got.Payload)
	}
}

func TestDecodeBrowserCallNormalizesEmptyPayload(t *testing.T) {
	for _, payload := range []string{``, `, "payload": null`, `, "payload": {}`} {
		input := `{"version":1,"id":"ui-call-1","type":"req","capability":"llm","method":"complete"` + payload + `}`
		got, err := decodeBrowserCall(strings.NewReader(input))
		if err != nil {
			t.Fatalf("input %q: %v", input, err)
		}
		if string(got.Payload) != "{}" {
			t.Fatalf("payload = %s, want {}", got.Payload)
		}
	}
}

func TestDecodeBrowserCallRejectsInvalidInput(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "missing id",
			input: `{"version":1,"type":"req","capability":"llm","method":"complete","payload":{}}`,
			want:  "id",
		},
		{
			name:  "wrong version",
			input: `{"version":2,"id":"ui-call-1","type":"req","capability":"llm","method":"complete","payload":{}}`,
			want:  "version",
		},
		{
			name:  "wrong type",
			input: `{"version":1,"id":"ui-call-1","type":"evt","capability":"llm","method":"complete","payload":{}}`,
			want:  "type",
		},
		{
			name:  "missing capability",
			input: `{"version":1,"id":"ui-call-1","type":"req","method":"complete","payload":{}}`,
			want:  "capability",
		},
		{
			name:  "missing method",
			input: `{"version":1,"id":"ui-call-1","type":"req","capability":"llm","payload":{}}`,
			want:  "method",
		},
		{
			name:  "from forbidden",
			input: `{"version":1,"id":"ui-call-1","type":"req","from":"host","capability":"llm","method":"complete","payload":{}}`,
			want:  "from",
		},
		{
			name:  "null from forbidden",
			input: `{"version":1,"id":"ui-call-1","type":"req","from":null,"capability":"llm","method":"complete","payload":{}}`,
			want:  "from",
		},
		{
			name:  "unknown field",
			input: `{"version":1,"id":"ui-call-1","type":"req","capability":"llm","method":"complete","payload":{},"target":"session"}`,
			want:  "target",
		},
		{
			name:  "trailing json",
			input: `{"version":1,"id":"ui-call-1","type":"req","capability":"llm","method":"complete","payload":{}} {}`,
			want:  "one",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeBrowserCall(strings.NewReader(test.input))
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want substring %q", err, test.want)
			}
		})
	}
}

func TestBrowserFrameRewritesIDWithoutMutatingSource(t *testing.T) {
	for _, frameType := range []string{protocol.FrameEvent, protocol.FrameResponse} {
		source := &protocol.Frame{
			Version:    protocol.FrameVersion,
			ID:         "host-1",
			Type:       frameType,
			From:       "session",
			Capability: "session",
			Method:     "query",
			Payload:    json.RawMessage(`{"ok":true}`),
		}

		got, err := browserFrame(source, "ui-call-7")
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != "ui-call-7" || source.ID != "host-1" {
			t.Fatalf("IDs = got:%s source:%s", got.ID, source.ID)
		}
		if got.Type != frameType || got.From != "session" || got.Capability != "session" || got.Method != "query" {
			t.Fatalf("frame = %#v", got)
		}
		if string(got.Payload) != `{"ok":true}` {
			t.Fatalf("payload = %s", got.Payload)
		}
	}
}

func TestBrowserFrameRejectsInvalidInput(t *testing.T) {
	if _, err := browserFrame(nil, "ui-call-1"); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := browserFrame(&protocol.Frame{
		Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameRequest,
	}, "ui-call-1"); err == nil {
		t.Fatal("request source accepted")
	}
	if _, err := browserFrame(&protocol.Frame{
		Version: protocol.FrameVersion, ID: "host-1", Type: protocol.FrameResponse,
	}, ""); err == nil {
		t.Fatal("empty browser id accepted")
	}
}

