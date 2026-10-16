package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// ExecutionContext is a Host-managed scheduling scope. Plugin business terms such
// as session or subagent remain outside Host; Host only tracks identity and bindings.
type ExecutionContext struct {
	ID           string            `json:"id"`
	Owner        string            `json:"owner"`
	RootID       string            `json:"root_id"`
	ParentID     string            `json:"parent_id,omitempty"`
	Bindings     map[string]string `json:"bindings,omitempty"`
	Participants map[string]bool   `json:"-"`
	Closed       bool              `json:"closed"`
}

// PluginInstanceSnapshot is the runtime view of one plugin process instance.
type PluginInstanceSnapshot struct {
	InstanceID     string `json:"instance_id"`
	PluginName     string `json:"plugin_name"`
	ConfigRevision string `json:"config_revision,omitempty"`
	State          string `json:"state"`
	Generation     int    `json:"generation"`
}

const (
	PluginInstanceStateMounted  = "mounted"
	PluginInstanceStateDegraded = "degraded"
)

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
		return PluginInstanceSnapshot{State: PluginInstanceStateDegraded}
	}
	state := PluginInstanceStateDegraded
	if p.healthy {
		state = PluginInstanceStateMounted
	}
	return PluginInstanceSnapshot{
		InstanceID:     p.instanceID,
		PluginName:     p.pluginName,
		ConfigRevision: p.configRevision,
		State:          state,
		Generation:     p.gen,
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
		code := errorCodeFor(err, CodeHandlerError)
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
		code := errorCodeFor(err, CodeHandlerError)
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
		code := errorCodeFor(err, CodeHandlerError)
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
		code := errorCodeFor(err, CodeHandlerError)
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
		code := errorCodeFor(err, CodeHandlerError)
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
		code := errorCodeFor(err, CodeHandlerError)
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
	if p.stopping {
		h.mu.Unlock()
		return nil
	}
	p.stopping = true
	h.mu.Unlock()

	stopProc(p)
	h.mu.Lock()
	if current := h.plugins[instanceID]; current == p {
		delete(h.plugins, instanceID)
	}
	h.mu.Unlock()
	failed := h.failInstancePending(instanceID, CodePluginDown, "plugin instance closed")
	h.hlog(fmt.Sprintf("instance closed instance=%s pending_failed=%d", instanceID, failed))
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
	if current := h.plugins[instanceID]; current != p || p.stopping {
		h.mu.Unlock()
		return PluginInstanceSnapshot{}, fmt.Errorf("instance %s: %w", instanceID, ErrInstanceClosed)
	}
	h.mu.Unlock()

	stopProc(p)
	h.mu.Lock()
	if current := h.plugins[instanceID]; current == p {
		delete(h.plugins, instanceID)
	}
	h.mu.Unlock()
	failed := h.failInstancePending(instanceID, CodePluginDown, "plugin instance restarting")

	if err := h.launchInstance(instanceID, discovery, configRevision); err != nil {
		return PluginInstanceSnapshot{}, err
	}
	h.mu.RLock()
	snapshot := instanceSnapshot(h.plugins[instanceID])
	h.mu.RUnlock()
	h.hlog(fmt.Sprintf("instance restarted instance=%s old_generation=%d generation=%d pending_failed=%d",
		instanceID, p.gen, snapshot.Generation, failed))
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
		Bindings:     make(map[string]string, len(bindings)),
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
		if ownerPlugin := h.routeOwnerLocked(capability, ""); ownerPlugin != "" && ownerPlugin != target.pluginName {
			return ExecutionContext{}, fmt.Errorf("instance %s does not own %s: %w", instanceID, capability, ErrContextBinding)
		}
		context.Bindings[capability] = instanceID
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
		for capability, instanceID := range parent.Bindings {
			if _, overridden := context.Bindings[capability]; !overridden {
				context.Bindings[capability] = instanceID
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
	if ownerPlugin := h.routeOwnerLocked(capability, ""); ownerPlugin != "" && ownerPlugin != target.pluginName {
		return fmt.Errorf("instance %s does not own %s: %w", instanceID, capability, ErrContextBinding)
	}
	context.Bindings[capability] = instanceID
	h.hlog(fmt.Sprintf("context bound context=%s capability=%s instance=%s", contextID, capability, instanceID))
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
	context.Bindings = make(map[string]string, len(bindings))
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
		if instanceID := context.Bindings[capability]; instanceID != "" {
			h.mu.RLock()
			p := h.plugins[instanceID]
			h.mu.RUnlock()
			if p == nil {
				return "", fmt.Errorf("bound instance %s: %w", instanceID, ErrInstanceNotFound)
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

func errorCodeFor(err error, fallback string) string {
	switch {
	case errors.Is(err, ErrPluginNotMounted):
		return CodePluginNotMounted
	case errors.Is(err, ErrPluginDown):
		return CodePluginDown
	case errors.Is(err, ErrPluginDisabled):
		return CodePluginDisabled
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

func (h *Host) failInstancePending(instanceID, code, message string) int {
	h.mu.Lock()
	var failed []*wait
	for id, wt := range h.pending {
		if wt.target != instanceID && wt.caller != instanceID {
			continue
		}
		delete(h.pending, id)
		if wt.target == instanceID {
			failed = append(failed, wt)
		}
	}
	h.mu.Unlock()

	for _, wt := range failed {
		h.failWait(wt, code, message)
	}
	return len(failed)
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
	if wt.kind == WaitHost && wt.resCh != nil {
		select {
		case wt.resCh <- &Frame{
			Version:    FrameVersion,
			ID:         wt.frameID,
			Type:       FrameResponse,
			Capability: wt.capability,
			Method:     wt.method,
			ContextID:  wt.contextID,
			From:       FromHost,
			ErrorCode:  code,
			ErrorMsg:   message,
		}:
		default:
		}
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
		<-p.done
	}
}
