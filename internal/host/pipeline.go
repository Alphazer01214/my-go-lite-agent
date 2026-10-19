package host

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

type pipelineState struct {
	mu           sync.Mutex
	capability   string
	method       string
	contextID    string
	nextPriority int
	hops         int
	visited      map[string]bool
	proceeded    map[string]bool
}

type pipelineCandidate struct {
	instanceID string
	pluginName string
	priority   int
	generation int
}

func newPipelineState(contextID, capability, method string) *pipelineState {
	return &pipelineState{
		capability:   capability,
		method:       method,
		contextID:    contextID,
		nextPriority: FramePriorityMax,
		visited:      make(map[string]bool),
		proceeded:    make(map[string]bool),
	}
}

func (h *Host) manifestForPlugin(pluginName string) (plugin.Manifest, bool) {
	for _, discovery := range h.discoveries.Plugins {
		if discovery.Manifest.Name == pluginName {
			return discovery.Manifest, true
		}
	}
	return plugin.Manifest{}, false
}

func consumesMethod(manifest plugin.Manifest, capability, method string) bool {
	for _, item := range manifest.Consumes {
		if item.Capability == capability && item.Method == method {
			return true
		}
	}
	return false
}

func providesCapability(manifest plugin.Manifest, capability string) bool {
	for _, provided := range manifest.Provides {
		if provided == capability {
			return true
		}
	}
	return false
}

func hasRegistration(p *proc, capability, method string) bool {
	for _, registration := range p.registrations {
		if registration.Capability == capability && registration.Method == method {
			return true
		}
	}
	return false
}

// selectPipelineCandidate chooses the highest eligible Consumer at or below
// the current pipeline Priority. Duplicate highest Priorities are explicit errors.
func (h *Host) selectPipelineCandidate(state *pipelineState) (pipelineCandidate, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.hops >= PipelineMaxHops {
		return pipelineCandidate{}, fmt.Errorf("pipeline hop limit %d: %w", PipelineMaxHops, ErrPipelineHops)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	key := state.capability + "." + state.method
	requireConsumer := state.hops == 0 && h.declaresConsumedMethodLocked(state.capability, state.method)
	var best pipelineCandidate
	found := false
	conflict := false
	visitedCandidate := false
	for _, p := range h.plugins {
		if p == nil || p.state != ProviderStateActive {
			continue
		}
		manifest, ok := h.manifestForPlugin(p.pluginName)
		if !ok {
			manifest = p.discovery.Manifest
		}
		if !hasRegistration(p, state.capability, state.method) && !consumesMethod(manifest, state.capability, state.method) {
			continue
		}
		if requireConsumer && !consumesMethod(manifest, state.capability, state.method) {
			continue
		}
		if state.visited[p.instanceID] {
			visitedCandidate = true
			continue
		}
		priority := manifest.Priority
		if priority > state.nextPriority {
			continue
		}
		if !found || priority > best.priority {
			best = pipelineCandidate{instanceID: p.instanceID, pluginName: p.pluginName, priority: priority, generation: p.gen}
			found = true
			conflict = false
			continue
		}
		if priority == best.priority {
			conflict = true
		}
	}
	if conflict {
		return pipelineCandidate{}, fmt.Errorf("duplicate Consumer Priority for %s: %w", key, ErrConsumePriority)
	}
	if !found {
		if visitedCandidate {
			return pipelineCandidate{}, fmt.Errorf("pipeline loop for %s: %w", key, ErrPipelineLoop)
		}
		return pipelineCandidate{}, fmt.Errorf("no Consumer for %s: %w", key, ErrMethodNotFound)
	}
	state.visited[best.instanceID] = true
	state.hops++
	state.nextPriority = best.priority - 1
	return best, nil
}

func (h *Host) declaresConsumedMethodLocked(capability, method string) bool {
	for _, discovery := range h.discoveries.Plugins {
		if consumesMethod(discovery.Manifest, capability, method) {
			return true
		}
	}
	return false
}

func (h *Host) hostProceed(from string, frame *Frame) error {
	var in struct {
		CallID  string          `json:"call_id"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad proceed payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}

	h.mu.Lock()
	current := h.pending[in.CallID]
	if current == nil || current.target != from || current.pipeline == nil {
		h.mu.Unlock()
		err := fmt.Errorf("proceed call %s is not pending: %w", in.CallID, ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	state := current.pipeline
	contextID := current.contextID
	capability := current.capability
	method := current.method
	payload := in.Payload
	if len(payload) == 0 {
		payload = current.payload
	}
	h.mu.Unlock()

	state.mu.Lock()
	if state.proceeded[in.CallID] {
		state.mu.Unlock()
		err := fmt.Errorf("call %s already proceeded: %w", in.CallID, ErrFlowProceeded)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeFlowAlreadyProceeded, err.Error())
		return err
	}
	state.proceeded[in.CallID] = true
	state.mu.Unlock()

	frameCopy := *frame
	go h.finishProceed(from, &frameCopy, state, contextID, capability, method, payload)
	return nil
}

// finishProceed runs the remainder of a Proceed outside the Host read loop.
// A Consumer's downstream call can enter the pipeline again, so blocking here
// would prevent the nested request from being routed.
func (h *Host) finishProceed(
	from string,
	frame *Frame,
	state *pipelineState,
	contextID, capability, method string,
	payload json.RawMessage,
) {
	candidate, err := h.selectPipelineCandidate(state)
	if err != nil {
		code := ErrorCode(err, CodeRouteFailed)
		h.replyError(from, frame.ID, capability, method, code, err.Error())
		return
	}

	h.mu.Lock()
	h.seq++
	nextID := fmt.Sprintf("%s-%d", ForwardIDPrefix, h.seq)
	resCh := make(chan *Frame, 1)
	nextWait := &wait{
		kind:             WaitHost,
		caller:           from,
		callerPlugin:     from,
		target:           candidate.instanceID,
		targetPlugin:     candidate.pluginName,
		targetGeneration: candidate.generation,
		targetPriority:   candidate.priority,
		pipeline:         state,
		contextID:        contextID,
		frameID:          frame.ID,
		capability:       capability,
		method:           method,
		payload:          append(json.RawMessage(nil), payload...),
		resCh:            resCh,
		onEvent: func(ev *Frame) {
			_ = h.write(from, ev)
		},
	}
	h.pending[nextID] = nextWait
	h.mu.Unlock()

	request := &Frame{
		Version:    FrameVersion,
		ID:         nextID,
		Type:       FrameRequest,
		From:       from,
		ContextID:  contextID,
		Capability: capability,
		Method:     method,
		Payload:    append(json.RawMessage(nil), payload...),
		Priority:   candidate.priority,
	}
	if err := h.write(candidate.instanceID, request); err != nil {
		h.mu.Lock()
		delete(h.pending, nextID)
		h.mu.Unlock()
		h.replyError(from, frame.ID, capability, method, CodeRouteFailed, err.Error())
		return
	}

	result, ok := <-resCh
	if !ok || result == nil {
		// fail()/Close() empty the waiter without a res; nothing else can arrive.
		h.replyError(from, frame.ID, capability, method, CodeServerClosed, "host closed")
		return
	}
	if result.ErrorCode != "" {
		h.replyError(from, frame.ID, capability, method, result.ErrorCode, result.ErrorMsg)
		return
	}
	h.replyOK(from, frame.ID, capability, method, result.Payload)
}
