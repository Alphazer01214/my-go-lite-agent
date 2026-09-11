package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestFrameRoundtrip(t *testing.T) {
	payload := json.RawMessage(`{"hello":"world"}`)
	in := &Frame{
		V:       1,
		ID:      "call-1",
		Type:    TypeReq,
		Cap:     "echo",
		Method:  "echo",
		Payload: payload,
	}

	var buf bytes.Buffer
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if out.V != in.V || out.ID != in.ID || out.Type != in.Type || out.Cap != in.Cap || out.Method != in.Method {
		t.Fatalf("header mismatch: %+v vs %+v", out, in)
	}
	if !bytes.Equal(out.Payload, payload) {
		t.Fatalf("payload mismatch: %s vs %s", out.Payload, payload)
	}
}

func TestReadFrameEOF(t *testing.T) {
	if _, err := ReadFrame(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

func TestReadFrameInvalidLength(t *testing.T) {
	// length 0 is rejected
	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 0})
	if _, err := ReadFrame(&buf); err == nil {
		t.Fatal("want error for zero length")
	}
}
