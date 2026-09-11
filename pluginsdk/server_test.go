package pluginsdk

import (
	"encoding/json"
	"testing"
)

func TestHandleDispatch(t *testing.T) {
	// Handler shape covered by Host integration tests; unit-test key mapping.
	s := New()
	s.Handle("echo", "echo", func(req *Request) (json.RawMessage, error) {
		return req.Payload, nil
	})
	s.mu.Lock()
	_, ok := s.handlers["echo.echo"]
	s.mu.Unlock()
	if !ok {
		t.Fatal("handler not registered")
	}
}
