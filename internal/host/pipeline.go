package host

import (
	"encoding/json"
	"errors"
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

// noActiveProviderError distinguishes a stopped provider from a real routing
// cycle. A stopped provider removes its route, so the pipeline only sees the
// already visited consumers and reports a loop. Name the outage instead.
func (h *Host) noActiveProviderError(capability, method string) error {
	key := capability + "." + method
	h.mu.RLock()
	owner, ok := h.routes[key]
	active := false
	if ok {
		if p := h.plugins[owner]; p != nil && p.state == ProviderStateActive {
			active = true
		}
	}
	h.mu.RUnlock()
	if active {
		return nil
	}
	return fmt.Errorf("no active provider for %s: %w", key, ErrPluginNotReady)
}

func (h *Host) declaresConsumedMethodLocked(capability, method string) bool {
	for _, discovery := range h.discoveries.Plugins {
		if consumesMethod(discovery.Manifest, capability, method) {
			return true
		}
	}
	return false
}

// proceedAck is the internal control Response to host.proceed. It only
// confirms the handoff and never carries a business result.
var proceedAck = json.RawMessage(`{"accepted":true}`)

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

	h.mu.RLock()
	current := h.pending[in.CallID]
	if current == nil || current.target != from || current.pipeline == nil {
		h.mu.RUnlock()
		err := fmt.Errorf("proceed call %s is not pending: %w", in.CallID, ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	state := current.pipeline
	h.mu.RUnlock()

	state.mu.Lock()
	if state.proceeded[in.CallID] {
		state.mu.Unlock()
		err := fmt.Errorf("call %s already proceeded: %w", in.CallID, ErrFlowProceeded)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeFlowAlreadyProceeded, err.Error())
		return err
	}
	state.proceeded[in.CallID] = true
	state.mu.Unlock()

	payload := in.Payload
	if len(payload) == 0 {
		payload = current.payload
	}

	frameCopy := *frame
	go h.finishProceed(from, &frameCopy, state, in.CallID, current, payload)
	return nil
}

// finishProceed retargets a proceeded request at the next hop outside the Host
// read loop. A Consumer's downstream call can enter the pipeline again, so
// blocking here would prevent the nested request from being routed.
//
// The continuation pending keeps the original caller and frame id: the terminal
// Response and attributed events bypass every Consumer and land on the original
// waiter. The Consumer only gets the internal control ack.
func (h *Host) finishProceed(
	from string,
	frame *Frame,
	state *pipelineState,
	callID string,
	proceeded *wait,
	payload json.RawMessage,
) {
	contextID := proceeded.contextID
	capability := proceeded.capability
	method := proceeded.method

	candidate, err := h.selectPipelineCandidate(state)
	if err != nil {
		if errors.Is(err, ErrPipelineLoop) {
			// Every Consumer has been visited. An active Route Owner is the
			// terminal hop; otherwise the outage is the real error.
			if providerErr := h.noActiveProviderError(capability, method); providerErr == nil {
				candidate, err = h.fallbackCandidate(state, contextID, capability, method)
			} else {
				err = providerErr
			}
		} else if isPipelineNoCandidate(err) {
			// 缺席即放行：没有下一个 Consumer 时交给 Route Owner。
			candidate, err = h.fallbackCandidate(state, contextID, capability, method)
		}
		if err != nil {
			h.failProceedAck(from, frame, err)
			return
		}
	}
	if err := h.acceptingNewCall(candidate.instanceID); err != nil {
		h.failProceedAck(from, frame, err)
		return
	}
	if err := h.joinExecutionContext(contextID, candidate.instanceID); err != nil {
		h.failProceedAck(from, frame, err)
		return
	}

	h.mu.Lock()
	if h.pending[callID] != proceeded {
		h.mu.Unlock()
		h.failProceedAck(from, frame, fmt.Errorf("proceed call %s is no longer pending: %w", callID, ErrMethodNotFound))
		return
	}
	// A cancel may have landed between the Proceed and this handoff; it must
	// follow the request to the new hop instead of dying with the old pending.
	cancelReason := proceeded.cancelReason
	h.seq++
	nextID := fmt.Sprintf("%s-%d", ForwardIDPrefix, h.seq)
	nextWait := &wait{
		// Only plugin-originated requests enter the pipeline, so the
		// continuation writes back to the original caller's instance.
		kind:             WaitPlugin,
		caller:           proceeded.caller,
		callerPlugin:     proceeded.callerPlugin,
		target:           candidate.instanceID,
		targetPlugin:     candidate.pluginName,
		targetGeneration: candidate.generation,
		targetPriority:   candidate.priority,
		pipeline:         state,
		contextID:        contextID,
		frameID:          proceeded.frameID,
		capability:       capability,
		method:           method,
		payload:          append(json.RawMessage(nil), payload...),
	}
	// The old pending must go now: a forward-path Response for it is stale,
	// and cancels from the original caller must reach the new hop instead.
	// Its cancel timer may stay armed; once the key is gone it is a no-op.
	delete(h.pending, callID)
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
		// Hand the request back to the old owner so the Consumer's error
		// Response still reaches the original caller.
		h.mu.Lock()
		if h.pending[nextID] == nextWait {
			delete(h.pending, nextID)
		}
		h.pending[callID] = proceeded
		h.mu.Unlock()
		h.failProceedAck(from, frame, err)
		return
	}
	if cancelReason != "" {
		_ = h.cancelPending(nextID, cancelReason)
	}

	h.replyOK(from, frame.ID, frame.Capability, frame.Method, proceedAck)
}

// failProceedAck reports a refused Proceed to the Consumer. The original
// pending stays in place, so the Consumer's error Response still completes the
// request through the normal dispatch path.
func (h *Host) failProceedAck(from string, frame *Frame, err error) {
	code := ErrorCode(err, CodeRouteFailed)
	h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
}

// fallbackCandidate resolves the Route Owner used when the pipeline has no
// eligible Consumer left. It advances state the same way handleRequest does,
// so loops and the hop limit stay consistent across both entry points.
func (h *Host) fallbackCandidate(state *pipelineState, contextID, capability, method string) (pipelineCandidate, error) {
	ownerPlugin := h.routeOwner(capability, method)
	if ownerPlugin == "" {
		return pipelineCandidate{}, fmt.Errorf("no owner for %s.%s: %w", capability, method, ErrMethodNotFound)
	}
	owner, err := h.resolveInstance(contextID, capability, method, ownerPlugin)
	if err != nil {
		return pipelineCandidate{}, err
	}
	h.mu.RLock()
	priority := 0
	generation := 0
	if p := h.plugins[owner]; p != nil {
		generation = p.gen
		if manifest, ok := h.manifestForPlugin(p.pluginName); ok {
			priority = manifest.Priority
		}
	}
	h.mu.RUnlock()
	state.mu.Lock()
	state.visited[owner] = true
	state.hops++
	state.nextPriority = priority - 1
	state.mu.Unlock()
	return pipelineCandidate{instanceID: owner, pluginName: ownerPlugin, priority: priority, generation: generation}, nil
}
