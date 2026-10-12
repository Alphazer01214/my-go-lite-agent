package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// Frame 线类型与编解码的唯一定义在 protocol；Host 经别名使用（payloads.go）。

type CallResult struct {
	Frame  *Frame
	Events []*Frame
}

func (h *Host) route(from string, frame *Frame) {
	switch frame.Type {
	case FrameRequest:
		h.handleRequest(from, frame)
	case FrameResponse:
		h.handleResponse(frame)
	case FrameEvent:
		h.handleEvent(from, frame)
	default:
		return
	}
}

// handleRequest only accept req with frame.ID = "plugin-xxx"
// Target is resolved from the (capability, method) route table; self-call is a legal loopback.
func (h *Host) handleRequest(from string, frame *Frame) error {
	if h.isClosed() {
		err := fmt.Errorf("host is closed: %w", ErrHostClosed)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeHostClosed, err.Error())
		return err
	}
	if frame.Capability == HostCapability {
		return h.dispatchHost(from, frame)
	}
	if frame.Capability == "" || frame.Method == "" {
		err := fmt.Errorf("capability and method are required: %w", ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	owner := h.routeOwner(frame.Capability, frame.Method)
	if owner == "" {
		err := fmt.Errorf("no owner for %s.%s: %w", frame.Capability, frame.Method, ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
	if err := h.forward(from, owner, frame); err != nil {
		code := CodeRouteFailed
		// map sentinel errors to stable wire codes
		switch {
		case errors.Is(err, ErrPluginNotMounted):
			code = CodePluginNotMounted
		case errors.Is(err, ErrPluginDown):
			code = CodePluginDown
		case errors.Is(err, ErrPluginDisabled):
			code = CodePluginDisabled
		}
		h.replyError(from, frame.ID, frame.Capability, frame.Method, code, err.Error())
		return err
	}
	return nil
}

// dispatchHost handles capability=host methods (protocol §5.1).
// 每个 req 恰好一条 res：未知 method 合成 method_not_found。
func (h *Host) dispatchHost(from string, frame *Frame) error {
	switch frame.Method {
	case HostMethodRegister:
		return h.hostRegister(from, frame)
	case HostMethodPlugins:
		return h.hostPlugins(from, frame)
	default:
		err := fmt.Errorf("no handler for host.%s: %w", frame.Method, ErrMethodNotFound)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeMethodNotFound, err.Error())
		return err
	}
}

// hostRegister 是路由表的唯一写入口（protocol §5.1）。
// 同插件重复上报同键 last-wins；跨插件冲突键进 rejected 且不改属主。
func (h *Host) hostRegister(from string, frame *Frame) error {
	var in struct {
		Handlers []struct {
			Capability string `json:"capability"`
			Method     string `json:"method"`
		} `json:"handlers"`
	}
	if err := json.Unmarshal(frame.Payload, &in); err != nil {
		err = fmt.Errorf("bad register payload: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeBadPayload, err.Error())
		return err
	}
	type routeKey struct {
		Capability string `json:"capability"`
		Method     string `json:"method"`
	}
	accepted := make([]routeKey, 0, len(in.Handlers))
	rejected := make([]routeKey, 0)

	h.mu.Lock()
	for _, hd := range in.Handlers {
		if hd.Capability == "" || hd.Method == "" {
			rejected = append(rejected, routeKey{hd.Capability, hd.Method})
			continue
		}
		key := hd.Capability + "." + hd.Method
		if owner, ok := h.routes[key]; ok && owner != from {
			rejected = append(rejected, routeKey{hd.Capability, hd.Method})
			continue
		}
		h.routes[key] = from
		accepted = append(accepted, routeKey{hd.Capability, hd.Method})
	}
	h.mu.Unlock()

	payload, err := json.Marshal(map[string]any{"accepted": accepted, "rejected": rejected})
	if err != nil {
		err = fmt.Errorf("marshal register result: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeHandlerError, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, payload)
	return nil
}

// hostPlugins 应答当前已挂载插件名列表（观测/依赖图用）。
func (h *Host) hostPlugins(from string, frame *Frame) error {
	h.mu.RLock()
	names := make([]string, 0, len(h.plugins))
	for name := range h.plugins {
		names = append(names, name)
	}
	h.mu.RUnlock()
	sort.Strings(names)

	payload, err := json.Marshal(map[string]any{"plugins": names})
	if err != nil {
		err = fmt.Errorf("marshal plugins result: %w", err)
		h.replyError(from, frame.ID, frame.Capability, frame.Method, CodeHandlerError, err.Error())
		return err
	}
	h.replyOK(from, frame.ID, frame.Capability, frame.Method, payload)
	return nil
}

// routeOwner resolves the owning plugin process name for (capability, method).
// routes（host.register 上报）优先，provides 回落——register 上报是异步的，回落兜住启动竞态。
// Internal only — never exposed as a Frame addressing field.
func (h *Host) routeOwner(capability string, method string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if owner, ok := h.routes[capability+"."+method]; ok {
		return owner
	}
	return h.provides[capability]
}

// handleResponse only accept res with frame.ID = "fwd-xxx"
func (h *Host) handleResponse(frame *Frame) error {
	h.mu.Lock()
	wt, ok := h.pending[frame.ID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("response for unknown frame: %s", frame.ID)
	}
	delete(h.pending, frame.ID)
	h.mu.Unlock()

	// id 改回 caller 视角；from 标被调方（溯源，不参与路由）
	res := *frame
	res.ID = wt.frameID
	res.From = wt.target

	// WaitHost：投 resCh（buffered 1，double 防），不写回插件
	if wt.kind == WaitHost && wt.resCh != nil {
		select {
		case wt.resCh <- &res:
		default:
		}
		return nil
	}
	return h.write(wt.caller, &res)
}

// handleEvent only accept EVENT with frame.ID = "plugin-xxx"
// 归因 evt（带 id）：命中 pending 后 id 改回 caller 视角再投递——
// WaitPlugin 写回调用方，WaitHost 走 onEvent 回调。未命中 log 后丢弃。
func (h *Host) handleEvent(from string, frame *Frame) error {
	if frame.ID == "" {
		return h.broadcast(from, frame)
	}
	h.mu.Lock()
	wt, ok := h.pending[frame.ID]
	if !ok {
		h.mu.Unlock()
		h.hlog(fmt.Sprintf("drop evt %s.%s for unknown id %s", frame.Capability, frame.Method, frame.ID))
		return nil
	}
	kind := wt.kind
	caller := wt.caller
	originalID := wt.frameID
	onEvent := wt.onEvent
	if kind == WaitHost && onEvent == nil {
		wt.events = append(wt.events, frame)
	}
	h.mu.Unlock()

	// id 改回 caller 视角；from 标 evt 来源插件
	ev := *frame
	ev.ID = originalID
	ev.From = from

	if kind == WaitHost {
		if onEvent != nil {
			onEvent(&ev)
		}
		return nil
	}
	return h.write(caller, &ev)
}

// replyError synthesizes a res to the **caller** by write
func (h *Host) replyError(caller string, originalID string, capability string,
	method string, code string, message string) {
	_ = h.write(caller, &Frame{
		Version:    FrameVersion,
		ID:         originalID,
		Type:       FrameResponse,
		Capability: capability,
		Method:     method,
		From:       FromHost,
		ErrorCode:  code,
		ErrorMsg:   message,
	})
}

// replyOK synthesizes a success res to the caller (host-local methods).
func (h *Host) replyOK(caller string, originalID string, capability string,
	method string, payload json.RawMessage) {
	_ = h.write(caller, &Frame{
		Version:    FrameVersion,
		ID:         originalID,
		Type:       FrameResponse,
		Capability: capability,
		Method:     method,
		From:       FromHost,
		Payload:    payload,
	})
}

// forward sends a req frame from one plugin to another, and waits for the response.
// owner is the resolved plugin process name (Host-internal; not a Frame field).
// Self-call (owner == from) is a legal loopback.
func (h *Host) forward(from string, owner string, frame *Frame) error {
	if err := h.alive(owner); err != nil {
		return err
	}

	h.mu.Lock()
	// Here handles the frame original plugin(from) sent
	// the frame is turned into a wait in pending, with the **fwd** key
	// the fwd key (fwd-xxxxxx) marks the frame is waiting for a response
	h.seq++
	wt := &wait{
		kind:       WaitPlugin,
		caller:     from,
		target:     owner,
		frameID:    frame.ID,
		capability: frame.Capability,
		method:     frame.Method,
		payload:    append(json.RawMessage(nil), frame.Payload...),
	}
	// pending[fwd-xxx] = wait{plugin-xxx}
	forwardID := fmt.Sprintf("fwd-%d", h.seq)
	h.pending[forwardID] = wt
	p := h.plugins[owner]
	h.mu.Unlock()

	if p == nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return fmt.Errorf("plugin %s not mounted: %w", owner, ErrPluginNotMounted)
	}

	// Here the frame is forwarded, with the fwd-xxx
	// from 注入真实来源插件名（protocol §3.5），插件无法伪造
	forwardFrame := *frame
	forwardFrame.ID = forwardID
	forwardFrame.From = from

	if err := h.write(owner, &forwardFrame); err != nil {
		h.mu.Lock()
		delete(h.pending, forwardID)
		h.mu.Unlock()
		return err
	}
	return nil

}

// broadcast handles unattributed evt (no id).
func (h *Host) broadcast(from string, frame *Frame) error {
	h.hlog(fmt.Sprintf("drop broadcast evt %s.%s from %s", frame.Capability, frame.Method, from))
	return nil
}

func (h *Host) isClosed() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.closed
}

func (h *Host) write(to string, frame *Frame) error {

	if err := h.alive(to); err != nil {
		return err
	}
	h.mu.Lock()
	plg := h.plugins[to]
	plg.mu.Lock()
	err := WriteFrame(plg.stdin, frame)
	plg.mu.Unlock()
	h.mu.Unlock()
	if err != nil {
		return err
	}
	// 锁外埋点：trace 回调不得持锁调用
	h.tracef("out", to, frame)
	return nil
}

func (h *Host) read(name string, gen int, stdout io.Reader) {
	for {
		frame, err := ReadFrame(stdout)
		if err != nil {
			// Close 关 stdin 后子进程退出属正常收尾，不当作错误刷日志
			if !h.isClosed() {
				h.hlog(fmt.Sprintf("read frame error: %v", err))
			}
			h.markUnhealthy(name, gen)
			return
		}
		h.tracef("in", name, frame)
		h.route(name, frame)
	}
}
