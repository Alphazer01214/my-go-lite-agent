package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// ExecutionContext is a Host-managed scheduling scope. Plugin business terms such
// as session or subagent remain outside Host; Host only tracks identity and bindings.
// ContextBinding pins one Capability to one process generation.
type ContextBinding struct {
	Capability string `json:"capability"`
	InstanceID string `json:"instance_id"`
	Generation int    `json:"generation"`
}

type ExecutionContext struct {
	ID           string                    `json:"id"`
	Owner        string                    `json:"owner"`
	RootID       string                    `json:"root_id"`
	ParentID     string                    `json:"parent_id,omitempty"`
	Bindings     map[string]ContextBinding `json:"bindings,omitempty"`
	Participants map[string]bool           `json:"-"`
	Closed       bool                      `json:"closed"`
}

// PluginInstanceSnapshot is the runtime view of one plugin process instance.
// RegistrationSnapshot is one reported (capability, method) route.
// Name and Description come from the plugin's WithInfo and are display-only.
type RegistrationSnapshot struct {
	Capability  string `json:"capability"`
	Method      string `json:"method"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type PluginInstanceSnapshot struct {
	InstanceID     string                 `json:"instance_id"`
	PluginName     string                 `json:"plugin_name"`
	ConfigRevision string                 `json:"config_revision,omitempty"`
	State          string                 `json:"state"`
	Generation     int                    `json:"generation"`
	Registrations  []RegistrationSnapshot `json:"registrations,omitempty"`
	FailureCode    string                 `json:"failure_code,omitempty"`
	FailureMessage string                 `json:"failure_message,omitempty"`
}

var (
	ErrInstanceNotFound   = errors.New(CodeInstanceNotFound)
	ErrInstanceExists     = errors.New(CodeInstanceExists)
	ErrInstanceClosed     = errors.New(CodeInstanceClosed)
	ErrContextNotFound    = errors.New(CodeContextNotFound)
	ErrContextClosed      = errors.New(CodeContextClosed)
	ErrContextOwner       = errors.New(CodeContextOwnerMismatch)
	ErrContextParticipant = errors.New(CodeContextParticipantMismatch)
	ErrContextBinding     = errors.New(CodeContextBindingInvalid)
)

func instanceSnapshot(p *proc) PluginInstanceSnapshot {
	if p == nil {
		return PluginInstanceSnapshot{State: ProviderStateInactive}
	}
	return PluginInstanceSnapshot{
		InstanceID:     p.instanceID,
		PluginName:     p.pluginName,
		ConfigRevision: p.configRevision,
		State:          p.state,
		Generation:     p.gen,
		Registrations:  append([]RegistrationSnapshot(nil), p.registrations...),
		FailureCode:    p.failureCode,
		FailureMessage: p.failureMessage,
	}
}

// InstanceSnapshot returns deterministic runtime instance state.
func (h *Host) InstanceSnapshot() []PluginInstanceSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	items := make([]PluginInstanceSnapshot, 0, len(h.plugins))
	for _, p := range h.plugins {
		items = append(items, instanceSnapshot(p))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].InstanceID < items[j].InstanceID })
	return items
}

// hostContextCreate handles host.context.create for plugin instances.
func (h *Host) hostContextCreate(from string, frame *Frame) error {
	var in struct {
		ParentID string            `json:"parent_id"`
		Bindings map[string]string `json:"bindings"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad context create payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	context, err := h.CreateExecutionContext(from, in.ParentID, in.Bindings)
	if err != nil {
		code := ErrorCode(err, CodeHandlerError)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	payload, err := json.Marshal(map[string]any{"context": context})
	if err != nil {
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeHandlerError, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, payload)
	return nil
}

func (h *Host) hostContextBind(from string, frame *Frame) error {
	var in struct {
		ContextID  string `json:"context_id"`
		Capability string `json:"capability"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad context bind payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	if err := h.BindExecutionContext(from, in.ContextID, in.Capability, in.InstanceID); err != nil {
		code := ErrorCode(err, CodeHandlerError)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, json.RawMessage(`{"bound":true}`))
	return nil
}

func (h *Host) hostContextClose(from string, frame *Frame) error {
	var in struct {
		ContextID string `json:"context_id"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad context close payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	if err := h.CloseExecutionContext(from, in.ContextID); err != nil {
		code := ErrorCode(err, CodeHandlerError)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, json.RawMessage(`{"closed":true}`))
	return nil
}

func (h *Host) hostInstanceCreate(from string, frame *Frame) error {
	var in struct {
		PluginName     string `json:"plugin_name"`
		InstanceID     string `json:"instance_id"`
		ConfigRevision string `json:"config_revision"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad instance create payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	instance, err := h.CreatePluginInstance(in.PluginName, in.InstanceID, in.ConfigRevision)
	if err != nil {
		code := ErrorCode(err, CodeHandlerError)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	payload, err := json.Marshal(map[string]any{"instance": instance})
	if err != nil {
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeHandlerError, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, payload)
	return nil
}

func (h *Host) hostInstanceClose(from string, frame *Frame) error {
	var in struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad instance close payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	if err := h.ClosePluginInstance(in.InstanceID); err != nil {
		code := ErrorCode(err, CodeHandlerError)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, json.RawMessage(`{"closed":true}`))
	return nil
}

func (h *Host) hostInstanceRestart(from string, frame *Frame) error {
	var in struct {
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad instance restart payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	instance, err := h.RestartPluginInstance(in.InstanceID)
	if err != nil {
		code := ErrorCode(err, CodeHandlerError)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	payload, err := json.Marshal(map[string]any{"instance": instance})
	if err != nil {
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeHandlerError, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, payload)
	return nil
}

// CreatePluginInstance starts one additional instance of a discovered plugin.
// The default instance keeps the plugin name as its instance id.
func (h *Host) CreatePluginInstance(pluginName, instanceID, configRevision string) (PluginInstanceSnapshot, error) {
	if pluginName == "" {
		return PluginInstanceSnapshot{}, fmt.Errorf("plugin name is required: %w", ErrBadArguments)
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return PluginInstanceSnapshot{}, ErrHostClosed
	}
	if instanceID == "" {
		h.seq++
		instanceID = fmt.Sprintf("%s:instance:%d", pluginName, h.seq)
	}
	if _, exists := h.plugins[instanceID]; exists {
		h.mu.Unlock()
		return PluginInstanceSnapshot{}, fmt.Errorf("instance %s already exists: %w", instanceID, ErrInstanceExists)
	}
	var discovery plugin.Discovery
	found := false
	for _, candidate := range h.discoveries.Plugins {
		if candidate.Manifest.Name == pluginName {
			discovery = candidate
			found = true
			break
		}
	}
	if !found {
		h.mu.Unlock()
		return PluginInstanceSnapshot{}, fmt.Errorf("plugin %s is not discovered: %w", pluginName, ErrPluginNotMounted)
	}
	if h.disabled[pluginName] {
		h.mu.Unlock()
		return PluginInstanceSnapshot{}, fmt.Errorf("plugin %s is disabled: %w", pluginName, ErrPluginDisabled)
	}
	h.mu.Unlock()

	if err := h.launchInstance(instanceID, discovery, configRevision); err != nil {
		return PluginInstanceSnapshot{}, err
	}
	h.mu.RLock()
	snapshot := instanceSnapshot(h.plugins[instanceID])
	h.mu.RUnlock()
	h.hlog(fmt.Sprintf("instance created plugin=%s instance=%s config_revision=%s generation=%d",
		snapshot.PluginName, snapshot.InstanceID, snapshot.ConfigRevision, snapshot.Generation))
	return snapshot, nil
}

// ClosePluginInstance stops one instance without changing other instances.
func (h *Host) ClosePluginInstance(instanceID string) error {
	h.mu.Lock()
	p := h.plugins[instanceID]
	if p == nil {
		h.mu.Unlock()
		return fmt.Errorf("instance %s: %w", instanceID, ErrInstanceNotFound)
	}
	if p.state == ProviderStateStopping {
		h.mu.Unlock()
		return nil
	}
	p.state = ProviderStateStopping
	h.mu.Unlock()

	if !h.waitForPendingTargets([]string{instanceID}, DefaultShutdownGrace) {
		h.failInstancePending(instanceID, CodePluginDown, "plugin instance closed before Call completed")
	}
	stopProc(p)
	h.markUnhealthy(instanceID, p.gen)
	h.mu.Lock()
	if current := h.plugins[instanceID]; current == p {
		delete(h.plugins, instanceID)
	}
	h.mu.Unlock()
	h.hlog(fmt.Sprintf("instance closed instance=%s", instanceID))
	return nil
}

// RestartPluginInstance replaces one process while preserving its instance id.
func (h *Host) RestartPluginInstance(instanceID string) (PluginInstanceSnapshot, error) {
	h.mu.RLock()
	p := h.plugins[instanceID]
	if p == nil {
		h.mu.RUnlock()
		return PluginInstanceSnapshot{}, fmt.Errorf("instance %s: %w", instanceID, ErrInstanceNotFound)
	}
	discovery := p.discovery
	configRevision := p.configRevision
	h.mu.RUnlock()

	h.mu.Lock()
	if current := h.plugins[instanceID]; current != p || p.state == ProviderStateStopping {
		h.mu.Unlock()
		return PluginInstanceSnapshot{}, fmt.Errorf("instance %s: %w", instanceID, ErrInstanceClosed)
	}
	p.state = ProviderStateStopping
	h.mu.Unlock()

	if !h.waitForPendingTargets([]string{instanceID}, DefaultShutdownGrace) {
		h.failInstancePending(instanceID, CodePluginDown, "plugin instance restarting before Call completed")
	}
	stopProc(p)
	h.markUnhealthy(instanceID, p.gen)
	h.mu.Lock()
	if current := h.plugins[instanceID]; current == p {
		delete(h.plugins, instanceID)
	}
	h.mu.Unlock()

	if err := h.launchInstance(instanceID, discovery, configRevision); err != nil {
		return PluginInstanceSnapshot{}, err
	}
	h.mu.RLock()
	snapshot := instanceSnapshot(h.plugins[instanceID])
	h.mu.RUnlock()
	h.hlog(fmt.Sprintf("instance restarted instance=%s old_generation=%d generation=%d",
		instanceID, p.gen, snapshot.Generation))
	return snapshot, nil
}

// CreateExecutionContext creates a generic scheduling scope. Parent identity is
// opaque to Host; Host does not interpret session or subagent semantics.
func (h *Host) CreateExecutionContext(owner, parentID string, bindings map[string]string) (ExecutionContext, error) {
	if owner == "" {
		return ExecutionContext{}, fmt.Errorf("owner is required: %w", ErrBadArguments)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ExecutionContext{}, ErrHostClosed
	}
	if h.contexts == nil {
		h.contexts = make(map[string]*ExecutionContext)
	}
	context := ExecutionContext{
		Owner:        owner,
		Bindings:     make(map[string]ContextBinding, len(bindings)),
		Participants: map[string]bool{owner: true},
	}
	for capability, instanceID := range bindings {
		if capability == "" || instanceID == "" {
			return ExecutionContext{}, fmt.Errorf("binding capability and instance are required: %w", ErrContextBinding)
		}
		target := h.plugins[instanceID]
		if target == nil {
			return ExecutionContext{}, fmt.Errorf("binding instance %s: %w", instanceID, ErrInstanceNotFound)
		}
		if h.hasCapabilityRouteLocked(capability) && !h.ownsCapabilityLocked(target.pluginName, capability) {
			return ExecutionContext{}, fmt.Errorf("instance %s does not own %s: %w", instanceID, capability, ErrContextBinding)
		}
		context.Bindings[capability] = ContextBinding{
			Capability: capability,
			InstanceID: instanceID,
			Generation: target.gen,
		}
	}
	if parentID != "" {
		parent, ok := h.contexts[parentID]
		if !ok {
			return ExecutionContext{}, fmt.Errorf("parent context %s: %w", parentID, ErrContextNotFound)
		}
		if parent.Closed {
			return ExecutionContext{}, fmt.Errorf("parent context %s: %w", parentID, ErrContextClosed)
		}
		if !parent.Participants[owner] && parent.Owner != owner {
			return ExecutionContext{}, fmt.Errorf("owner %s cannot create child of %s: %w", owner, parentID, ErrContextParticipant)
		}
		context.ParentID = parent.ID
		context.RootID = parent.RootID
		for capability, binding := range parent.Bindings {
			if _, overridden := context.Bindings[capability]; !overridden {
				context.Bindings[capability] = binding
			}
		}
		for participant := range parent.Participants {
			context.Participants[participant] = true
		}
	}
	h.contextSeq++
	context.ID = fmt.Sprintf("ctx-%d", h.contextSeq)
	if context.RootID == "" {
		context.RootID = context.ID
	}
	h.contexts[context.ID] = &context
	h.hlog(fmt.Sprintf("context created context=%s owner=%s root=%s parent=%s bindings=%d",
		context.ID, context.Owner, context.RootID, context.ParentID, len(context.Bindings)))
	return cloneExecutionContext(context), nil
}

// BindExecutionContext changes one capability binding for an owned context.
func (h *Host) BindExecutionContext(owner, contextID, capability, instanceID string) error {
	if owner == "" || contextID == "" || capability == "" || instanceID == "" {
		return fmt.Errorf("owner, context, capability and instance are required: %w", ErrBadArguments)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	context, ok := h.contexts[contextID]
	if !ok {
		return fmt.Errorf("context %s: %w", contextID, ErrContextNotFound)
	}
	if context.Closed {
		return fmt.Errorf("context %s: %w", contextID, ErrContextClosed)
	}
	if context.Owner != owner {
		return fmt.Errorf("owner %s: %w", owner, ErrContextOwner)
	}
	target := h.plugins[instanceID]
	if target == nil {
		return fmt.Errorf("instance %s: %w", instanceID, ErrInstanceNotFound)
	}
	if h.hasCapabilityRouteLocked(capability) && !h.ownsCapabilityLocked(target.pluginName, capability) {
		return fmt.Errorf("instance %s does not own %s: %w", instanceID, capability, ErrContextBinding)
	}
	context.Bindings[capability] = ContextBinding{
		Capability: capability,
		InstanceID: instanceID,
		Generation: target.gen,
	}
	h.hlog(fmt.Sprintf("context bound context=%s capability=%s instance=%s generation=%d",
		contextID, capability, instanceID, target.gen))
	return nil
}

// CloseExecutionContext closes one context subtree and fails its pending calls.
func (h *Host) CloseExecutionContext(owner, contextID string) error {
	h.mu.Lock()
	context, ok := h.contexts[contextID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("context %s: %w", contextID, ErrContextNotFound)
	}
	if context.Owner != owner {
		h.mu.Unlock()
		return fmt.Errorf("owner %s: %w", owner, ErrContextOwner)
	}

	closed := []string{contextID}
	for changed := true; changed; {
		changed = false
		for id, candidate := range h.contexts {
			if candidate.Closed || containsString(closed, id) || !containsString(closed, candidate.ParentID) {
				continue
			}
			candidate.Closed = true
			delete(h.contexts, id)
			closed = append(closed, id)
			changed = true
		}
	}
	context.Closed = true
	delete(h.contexts, contextID)
	h.mu.Unlock()

	for _, id := range closed {
		h.cancelPendingWhere(func(wt *wait) bool { return wt.contextID == id }, cancelReasonCancelled)
	}
	failed := 0
	for _, id := range closed {
		failed += h.failContextPending(id, CodeContextClosed, "execution context closed")
	}
	h.hlog(fmt.Sprintf("context closed context=%s contexts_closed=%d pending_failed=%d",
		contextID, len(closed), failed))
	return nil
}

// ExecutionContextSnapshot returns a copy suitable for callers.
func (h *Host) ExecutionContextSnapshot(contextID string) (ExecutionContext, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	context, ok := h.contexts[contextID]
	if !ok {
		return ExecutionContext{}, fmt.Errorf("context %s: %w", contextID, ErrContextNotFound)
	}
	return cloneExecutionContext(*context), nil
}

func cloneExecutionContext(context ExecutionContext) ExecutionContext {
	bindings := context.Bindings
	participants := context.Participants
	context.Bindings = make(map[string]ContextBinding, len(bindings))
	for key, value := range bindings {
		context.Bindings[key] = value
	}
	context.Participants = make(map[string]bool, len(participants))
	for key, value := range participants {
		context.Participants[key] = value
	}
	return context
}

func (h *Host) useExecutionContext(contextID, instanceID string) (ExecutionContext, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	context, ok := h.contexts[contextID]
	if !ok {
		return ExecutionContext{}, fmt.Errorf("context %s: %w", contextID, ErrContextNotFound)
	}
	if context.Closed {
		return ExecutionContext{}, fmt.Errorf("context %s: %w", contextID, ErrContextClosed)
	}
	if instanceID != "" && !context.Participants[instanceID] {
		return ExecutionContext{}, fmt.Errorf("instance %s: %w", instanceID, ErrContextParticipant)
	}
	if instanceID != "" {
		context.Participants[instanceID] = true
	}
	return cloneExecutionContext(*context), nil
}

func (h *Host) joinExecutionContext(contextID, instanceID string) error {
	if contextID == "" || instanceID == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	context, ok := h.contexts[contextID]
	if !ok {
		return fmt.Errorf("context %s: %w", contextID, ErrContextNotFound)
	}
	if context.Closed {
		return fmt.Errorf("context %s: %w", contextID, ErrContextClosed)
	}
	context.Participants[instanceID] = true
	return nil
}

func (h *Host) resolveInstance(contextID, capability, method, defaultPlugin string) (string, error) {
	if contextID != "" {
		context, err := h.useExecutionContext(contextID, "")
		if err != nil {
			return "", err
		}
		if binding := context.Bindings[capability]; binding.InstanceID != "" {
			instanceID := binding.InstanceID
			h.mu.RLock()
			p := h.plugins[instanceID]
			h.mu.RUnlock()
			if p == nil {
				return "", fmt.Errorf("bound instance %s: %w", instanceID, ErrInstanceNotFound)
			}
			if p.gen != binding.Generation {
				return "", fmt.Errorf("bound instance %s generation %d, current %d: %w",
					instanceID, binding.Generation, p.gen, ErrContextBinding)
			}
			if ownerPlugin := h.routeOwner(capability, method); ownerPlugin != "" && ownerPlugin != p.pluginName {
				return "", fmt.Errorf("bound instance %s does not own %s.%s: %w", instanceID, capability, method, ErrContextBinding)
			}
			return instanceID, nil
		}
	}
	if defaultPlugin == "" {
		return "", fmt.Errorf("no owner for %s.%s: %w", capability, method, ErrMethodNotFound)
	}
	return defaultPlugin, nil
}

// ErrorCode maps Host sentinel errors to stable wire error codes.
func ErrorCode(err error, fallback string) string {
	switch {
	case errors.Is(err, ErrMethodNotFound):
		return CodeMethodNotFound
	case errors.Is(err, ErrHostClosed):
		return CodeHostClosed
	case errors.Is(err, ErrServerClosed):
		return CodeServerClosed
	case errors.Is(err, ErrBadArguments):
		return CodeBadArguments
	case errors.Is(err, ErrBadPayload):
		return CodeBadPayload
	case errors.Is(err, ErrTimeout):
		return CodeTimeout
	case errors.Is(err, ErrRouteFailed):
		return CodeRouteFailed
	case errors.Is(err, ErrPluginNotMounted):
		return CodePluginNotMounted
	case errors.Is(err, ErrPluginDown):
		return CodePluginDown
	case errors.Is(err, ErrPluginDisabled):
		return CodePluginDisabled
	case errors.Is(err, ErrPluginNotReady):
		return CodePluginNotReady
	case errors.Is(err, ErrConsumePriority):
		return CodeConsumePriorityConflict
	case errors.Is(err, ErrFlowProceeded):
		return CodeFlowAlreadyProceeded
	case errors.Is(err, ErrPipelineLoop):
		return CodePipelineLoop
	case errors.Is(err, ErrPipelineHops):
		return CodePipelineHopsExceeded
	case errors.Is(err, context.DeadlineExceeded):
		return CodeCallDeadlineExceeded
	case errors.Is(err, context.Canceled):
		return CodeCallCancelled
	case errors.Is(err, ErrContextNotFound):
		return CodeContextNotFound
	case errors.Is(err, ErrContextClosed):
		return CodeContextClosed
	case errors.Is(err, ErrContextOwner):
		return CodeContextOwnerMismatch
	case errors.Is(err, ErrContextParticipant):
		return CodeContextParticipantMismatch
	case errors.Is(err, ErrContextBinding):
		return CodeContextBindingInvalid
	case errors.Is(err, ErrInstanceNotFound):
		return CodeInstanceNotFound
	case errors.Is(err, ErrInstanceExists):
		return CodeInstanceExists
	case errors.Is(err, ErrInstanceClosed):
		return CodeInstanceClosed
	default:
		return fallback
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (h *Host) pluginNameForInstance(instanceID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if p := h.plugins[instanceID]; p != nil && p.pluginName != "" {
		return p.pluginName
	}
	return instanceID
}

// failInstancePending closes every pending that the dead instance takes part in.
// Waits targeted at it are answered with code/message. Waits it originated are
// also resolved, but only as a local signal: a WaitHost waiter is parked on
// resCh in Host's own goroutine and would otherwise block forever, while a
// WaitPlugin caller is a dead process that can no longer read a written back res.
// Either way the pending entry is removed so nothing waits on an id that will
// never be answered again.
func (h *Host) failInstancePending(instanceID, code, message string) int {
	h.mu.Lock()
	var targeted, orphaned []*wait
	for id, wt := range h.pending {
		if wt.target != instanceID && wt.caller != instanceID {
			continue
		}
		delete(h.pending, id)
		if wt.target == instanceID {
			targeted = append(targeted, wt)
			continue
		}
		if wt.kind == WaitHost {
			orphaned = append(orphaned, wt)
		}
	}
	h.mu.Unlock()

	for _, wt := range targeted {
		h.failWait(wt, code, message)
	}
	for _, wt := range orphaned {
		stopWaitTimer(wt)
		h.signalWaitResult(wt, &Frame{
			Version:    FrameVersion,
			ID:         wt.frameID,
			Type:       FrameResponse,
			Capability: wt.capability,
			Method:     wt.method,
			ContextID:  wt.contextID,
			From:       FromHost,
			ErrorCode:  code,
			ErrorMsg:   message,
		})
	}
	return len(targeted)
}

// signalWaitResult delivers one res to a WaitHost waiter. resCh is buffered 1 and
// every closer writes exactly once, so a full channel means the res already
// arrived and this one is redundant.
func (h *Host) signalWaitResult(wt *wait, res *Frame) {
	if wt == nil || wt.resCh == nil {
		return
	}
	select {
	case wt.resCh <- res:
	default:
	}
}

func (h *Host) failContextPending(contextID, code, message string) int {
	h.mu.Lock()
	var failed []*wait
	for id, wt := range h.pending {
		if wt.contextID != contextID {
			continue
		}
		delete(h.pending, id)
		failed = append(failed, wt)
	}
	h.mu.Unlock()

	for _, wt := range failed {
		h.failWait(wt, code, message)
	}
	return len(failed)
}

func (h *Host) failWait(wt *wait, code, message string) {
	if wt == nil {
		return
	}
	stopWaitTimer(wt)
	if wt.kind == WaitHost {
		h.signalWaitResult(wt, &Frame{
			Version:    FrameVersion,
			ID:         wt.frameID,
			Type:       FrameResponse,
			Capability: wt.capability,
			Method:     wt.method,
			ContextID:  wt.contextID,
			From:       FromHost,
			ErrorCode:  code,
			ErrorMsg:   message,
		})
		return
	}
	h.replyError(wt.caller, wt.frameID, wt.capability, wt.method, code, message)
}

func stopProc(p *proc) {
	if p == nil {
		return
	}
	_ = p.stdin.Close()
	timer := time.NewTimer(DefaultShutdownGrace)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		select {
		case <-p.done:
		case <-time.After(DefaultShutdownGrace):
		}
	}
}
