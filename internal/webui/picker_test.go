package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPickDirectoryReturnsPath(t *testing.T) {
	want := t.TempDir()
	server := NewServer(nil, nil)
	server.pickDirectory = func(context.Context) (string, bool, error) {
		return want, false, nil
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/bridge/pick-directory", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out pickDirectoryOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Path != want || out.Cancelled {
		t.Fatalf("response = %#v", out)
	}
}

func TestPickDirectoryCancelled(t *testing.T) {
	server := NewServer(nil, nil)
	server.pickDirectory = func(context.Context) (string, bool, error) {
		return "", true, nil
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/bridge/pick-directory", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out pickDirectoryOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Cancelled || out.Path != "" {
		t.Fatalf("response = %#v", out)
	}
}

func TestPickDirectoryErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "unsupported",
			err:    errDirectoryPickerUnsupported,
			status: http.StatusNotImplemented,
			code:   "folder_picker_unsupported",
		},
		{
			name:   "failed",
			err:    errors.New("dialog failed"),
			status: http.StatusInternalServerError,
			code:   "folder_picker_failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := NewServer(nil, nil)
			server.pickDirectory = func(context.Context) (string, bool, error) {
				return "", false, test.err
			}

			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/bridge/pick-directory", nil))
			if rec.Code != test.status {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var payload struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error.Code != test.code || payload.Error.Message == "" {
				t.Fatalf("error = %#v", payload.Error)
			}
		})
	}
}

func TestPickDirectoryRejectsCrossOrigin(t *testing.T) {
	server := NewServer(nil, nil)
	server.pickDirectory = func(context.Context) (string, bool, error) {
		return t.TempDir(), false, nil
	}

	request := httptest.NewRequest(http.MethodPost, "/bridge/pick-directory", nil)
	request.Header.Set("Origin", "https://example.invalid")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, request)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestDecodeWindowsPickerOutput(t *testing.T) {
	want := t.TempDir()
	encoded := base64.StdEncoding.EncodeToString([]byte(want))
	path, cancelled, err := decodeWindowsPickerOutput(encoded + "\r\n")
	if err != nil || cancelled || path != want {
		t.Fatalf("decode = %q, %v, %v", path, cancelled, err)
	}

	if _, cancelled, err := decodeWindowsPickerOutput(""); err != nil || !cancelled {
		t.Fatalf("empty output = cancelled %v, err %v", cancelled, err)
	}
	if _, _, err := decodeWindowsPickerOutput("not base64!"); err == nil {
		t.Fatal("invalid output was accepted")
	}
}
