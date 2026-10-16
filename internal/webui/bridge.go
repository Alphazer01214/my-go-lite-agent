package webui

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type CallBackend interface {
	CallWithEvents(
		from string,
		capability string,
		method string,
		payload json.RawMessage,
		onEvent func(*protocol.Frame),
	) (*protocol.Frame, error)
}

func serveBrowserCall(w http.ResponseWriter, r *http.Request, backend CallBackend) {
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
		writeBrowserResponse(w, request.ID, callErrorResponse(request, host.ErrMethodNotFound))
		return
	}

	done := make(chan struct{})
	defer close(done)
	queue := make(chan *protocol.Frame, 64)

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
	sendFinal := func(frame *protocol.Frame) {
		out, err := browserFrame(frame, request.ID)
		if err != nil {
			out = callErrorResponse(request, err)
		}
		select {
		case queue <- out:
		case <-done:
		}
	}

	go func() {
		frame, callErr := backend.CallWithEvents(
			"webui",
			request.Capability,
			request.Method,
			request.Payload,
			sendEvent,
		)
		if callErr != nil {
			sendFinal(callErrorResponse(request, callErr))
			return
		}
		sendFinal(frame)
	}()

	for {
		select {
		case frame := <-queue:
			if !writeBrowserResponse(w, request.ID, frame) {
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

func writeBrowserResponse(w http.ResponseWriter, requestID string, frame *protocol.Frame) bool {
	out, err := browserFrame(frame, requestID)
	if err != nil {
		http.Error(w, "failed to encode browser response", http.StatusInternalServerError)
		return false
	}
	if err := json.NewEncoder(w).Encode(out); err != nil {
		return false
	}
	return http.NewResponseController(w).Flush() == nil
}

func callErrorResponse(request browserCall, err error) *protocol.Frame {
	return &protocol.Frame{
		Version:    protocol.FrameVersion,
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
	switch {
	case errors.Is(err, host.ErrMethodNotFound):
		return protocol.CodeMethodNotFound
	case errors.Is(err, host.ErrHostClosed):
		return protocol.CodeHostClosed
	case errors.Is(err, host.ErrHostServerClosed):
		return protocol.CodeHostServerClosed
	case errors.Is(err, host.ErrPluginNotMounted):
		return protocol.CodePluginNotMounted
	case errors.Is(err, host.ErrPluginDown):
		return protocol.CodePluginDown
	case errors.Is(err, host.ErrPluginDisabled):
		return protocol.CodePluginDisabled
	case errors.Is(err, host.ErrBadArguments):
		return protocol.CodeBadArguments
	case errors.Is(err, host.ErrBadPayload):
		return protocol.CodeBadPayload
	case errors.Is(err, host.ErrTimeout):
		return protocol.CodeTimeout
	case errors.Is(err, host.ErrRouteFailed):
		return protocol.CodeRouteFailed
	default:
		return protocol.CodeHandlerError
	}
}
