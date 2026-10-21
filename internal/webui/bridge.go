package webui

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type CallBackend interface {
	CallWithEvents(
		ctx context.Context,
		from string,
		capability string,
		method string,
		payload json.RawMessage,
		onEvent func(*protocol.Frame),
	) (*protocol.Frame, error)
}

func serveBrowserCall(w http.ResponseWriter, r *http.Request, backend CallBackend, onDiagnostic func(plugin.Diagnostic)) {
	if backend == nil {
		http.Error(w, "call backend unavailable", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, protocol.FrameMaxSize)
	request, err := decodeBrowserCall(r.Body)
	if err != nil {
		http.Error(w, "invalid browser call", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := http.NewResponseController(w).Flush(); err != nil {
		return
	}

	if request.Capability == protocol.HostCapability {
		rejected, err := browserFrame(callErrorResponse(request, host.ErrMethodNotFound), request.ID)
		if err != nil {
			return
		}
		_ = json.NewEncoder(w).Encode(rejected)
		return
	}

	done := make(chan struct{})
	defer close(done)
	queue := make(chan *protocol.Frame, 64)

	// Frames are normalized to browser shape exactly once, here. sendEvent drops
	// under backpressure so a slow browser cannot stall the plugin's read loop:
	// an evt is a progress signal and the final res is what the caller waits for.
	sendEvent := func(frame *protocol.Frame) {
		out, err := browserFrame(frame, request.ID)
		if err != nil {
			return
		}
		select {
		case queue <- out:
		case <-done:
		default:
		}
	}
	// sendFinal must not drop: it is the only res this request ever gets, so it
	// blocks until the writer takes it or the browser goes away. A frame that
	// cannot be represented becomes a synthetic error res rather than silence.
	sendFinal := func(frame *protocol.Frame) {
		out, err := browserFrame(frame, request.ID)
		if err != nil {
			out, err = browserFrame(callErrorResponse(request, err), request.ID)
			if err != nil {
				return
			}
		}
		select {
		case queue <- out:
		case <-done:
		}
	}

	// The browser is only an observer. A refresh or tab close aborts this HTTP
	// response, but the business call must keep running so a reload can recover
	// from persistent state. Host.Close cancels pending calls on shutdown.
	callCtx := context.WithoutCancel(r.Context())
	go func() {
		frame, callErr := backend.CallWithEvents(
			callCtx,
			"webui",
			request.Capability,
			request.Method,
			request.Payload,
			sendEvent,
		)
		if callErr != nil {
			if diagnostic, ok := callDiagnostic(request, callErrorCode(callErr), callErr.Error()); ok && onDiagnostic != nil {
				onDiagnostic(diagnostic)
			}
			sendFinal(callErrorResponse(request, callErr))
			return
		}
		if frame != nil && frame.ErrorCode != "" {
			if diagnostic, ok := callDiagnostic(request, frame.ErrorCode, frame.ErrorMsg); ok && onDiagnostic != nil {
				onDiagnostic(diagnostic)
			}
		}
		sendFinal(frame)
	}()

	for {
		select {
		case frame := <-queue:
			if err := json.NewEncoder(w).Encode(frame); err != nil {
				return
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				return
			}
			if frame.Type == protocol.FrameResponse {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func callErrorResponse(request browserCall, err error) *protocol.Frame {
	return &protocol.Frame{
		Version:    BrowserVersion,
		ID:         request.ID,
		Type:       protocol.FrameResponse,
		From:       host.FromHost,
		Capability: request.Capability,
		Method:     request.Method,
		ErrorCode:  callErrorCode(err),
		ErrorMsg:   err.Error(),
	}
}

func callErrorCode(err error) string {
	return host.ErrorCode(err, protocol.CodeHandlerError)
}

// callDiagnostic turns a failed browser call into a user-facing diagnostic.
// Only calls the browser asked for reach this path, so tool-call failures the
// agent absorbs mid-turn stay out of the panel, and an explicit stop is not a
// fault the user needs to see.
func callDiagnostic(request browserCall, code, message string) (plugin.Diagnostic, bool) {
	if code == "" || code == protocol.CodeCallCancelled {
		return plugin.Diagnostic{}, false
	}
	return plugin.Diagnostic{
		Plugin:  request.Capability,
		Path:    request.Capability + "." + request.Method,
		Code:    code,
		Message: message,
	}, true
}
