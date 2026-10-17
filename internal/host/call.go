package host

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// CallWithEvents starts one Host-initiated request and streams attributed events
// to onEvent until the final response. The callback is called from the plugin
// read loop and must be fast and non-blocking.
func (h *Host) CallWithEvents(
	from string,
	capability string,
	method string,
	payload json.RawMessage,
	onEvent func(*Frame),
) (*Frame, error) {
	return h.CallWithEventsInContext("", from, capability, method, payload, onEvent)
}

// CallWithEventsInContext binds a Host-initiated call to one execution context.
func (h *Host) CallWithEventsInContext(
	contextID string,
	from string,
	capability string,
	method string,
	payload json.RawMessage,
	onEvent func(*Frame),
) (*Frame, error) {
	if contextID != "" {
		if _, err := h.useExecutionContext(contextID, from); err != nil {
			return nil, err
		}
	}
	if h.isClosed() {
		return nil, fmt.Errorf("host is closed: %w", ErrHostClosed)
	}
	if from == "" || capability == "" || method == "" {
		return nil, fmt.Errorf("from, capability, and method are required: %w", ErrBadArguments)
	}
	if capability == HostCapability {
		return nil, fmt.Errorf("external call cannot invoke %s.%s: %w", capability, method, ErrMethodNotFound)
	}

	ownerPlugin := h.routeOwner(capability, method)
	if ownerPlugin == "" {
		return nil, fmt.Errorf("no owner for %s.%s: %w", capability, method, ErrMethodNotFound)
	}
	owner, err := h.resolveInstance(contextID, capability, method, ownerPlugin)
	if err != nil {
		return nil, err
	}
	if err := h.alive(owner); err != nil {
		return nil, err
	}

	h.mu.Lock()
	h.seq++
	callID := HostCallIDPrefix + strconv.Itoa(h.seq)
	response := make(chan *Frame, 1)
	pendingWait := &wait{
		kind:         WaitHost,
		caller:       from,
		callerPlugin: from,
		target:       owner,
		targetPlugin: ownerPlugin,
		contextID:    contextID,
		frameID:      callID,
		capability:   capability,
		method:       method,
		payload:      append(json.RawMessage(nil), payload...),
		resCh:        response,
		onEvent:      onEvent,
	}
	h.pending[callID] = pendingWait
	h.mu.Unlock()

	request := &Frame{
		Version:    FrameVersion,
		ID:         callID,
		Type:       FrameRequest,
		From:       from,
		ContextID:  contextID,
		Capability: capability,
		Method:     method,
		Payload:    append(json.RawMessage(nil), payload...),
	}
	if err := h.write(owner, request); err != nil {
		h.mu.Lock()
		delete(h.pending, callID)
		h.mu.Unlock()
		return nil, fmt.Errorf("call %s.%s: %w", capability, method, err)
	}

	return <-response, nil
}
