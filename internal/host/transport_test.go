package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// bufferWC is a capturing, thread-safe stdin for a fake proc.
// 测试轮询读取与调用 goroutine 写入并发，故需加锁。
type bufferWC struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *bufferWC) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *bufferWC) Close() error { return nil }

// snapshot copies the captured bytes (safe to decode while writes continue).
func (b *bufferWC) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, b.buf.Len())
	copy(out, b.buf.Bytes())
	return out
}

func newTestHost(names ...string) *Host {
	h := &Host{
		plugins:     make(map[string]*proc),
		pending:     make(map[string]*wait),
		routes:      make(map[string]string),
		cancelGrace: DefaultCancelGrace,
		disabled:    make(map[string]bool),
		gen:         make(map[string]int),
		hlog:        func(string) {},
	}
	for _, n := range names {
		h.gen[n] = 1
		done := make(chan struct{})
		close(done) // 假 proc 无真实进程，视为已退出
		h.plugins[n] = &proc{instanceID: n, pluginName: n, stdin: &bufferWC{}, gen: 1, state: ProviderStateActive, done: done}
	}
	return h
}

// drainFrames decodes all complete frames a plugin received on stdin.
// 允许尾部半帧（写入可能正在进行），下一轮轮询会补全。
func drainFrames(t *testing.T, h *Host, name string) []*Frame {
	t.Helper()
	r := bytes.NewReader(h.plugins[name].stdin.(*bufferWC).snapshot())
	var out []*Frame
	for r.Len() > 0 {
		f, err := ReadFrame(r)
		if err != nil {
			break
		}
		out = append(out, f)
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestHostRegisterAcceptsAndRejectsConflict(t *testing.T) {
	h := newTestHost("a", "b")

	req := &Frame{
		ID: "a-1", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
		Payload: mustJSON(map[string]any{"handlers": []map[string]string{
			{"capability": "loop", "method": "turn"},
			{"capability": "llm", "method": "complete"},
		}}),
	}
	if err := h.handleRequest("a", req); err != nil {
		t.Fatalf("register a: %v", err)
	}

	// b 抢 loop.turn（a 已注册）→ rejected；自己的键 → accepted
	req2 := &Frame{
		ID: "b-1", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
		Payload: mustJSON(map[string]any{"handlers": []map[string]string{
			{"capability": "loop", "method": "turn"},
			{"capability": "session", "method": "create"},
		}}),
	}
	if err := h.handleRequest("b", req2); err != nil {
		t.Fatalf("register b: %v", err)
	}

	// 路由表：loop.turn 仍归 a，session.create 归 b
	if got := h.routeOwner("loop", "turn"); got != "a" {
		t.Fatalf("routeOwner loop.turn = %q, want a", got)
	}
	if got := h.routeOwner("session", "create"); got != "b" {
		t.Fatalf("routeOwner session.create = %q, want b", got)
	}

	// 两条 res：id 还原、from=host、accepted/rejected 形状正确
	bFrames := drainFrames(t, h, "b")
	if len(bFrames) != 1 {
		t.Fatalf("b got %d frames, want 1", len(bFrames))
	}
	res := bFrames[0]
	if res.ID != "b-1" || res.From != FromHost || res.Type != FrameResponse {
		t.Fatalf("b res = id:%s from:%s type:%s", res.ID, res.From, res.Type)
	}
	var out struct {
		Accepted []map[string]string `json:"accepted"`
		Rejected []map[string]string `json:"rejected"`
	}
	if err := json.Unmarshal(res.Payload, &out); err != nil {
		t.Fatalf("unmarshal register res: %v", err)
	}
	if len(out.Accepted) != 1 || out.Accepted[0]["capability"] != "session" {
		t.Fatalf("accepted = %v, want only session.create", out.Accepted)
	}
	if len(out.Rejected) != 1 || out.Rejected[0]["capability"] != "loop" {
		t.Fatalf("rejected = %v, want only loop.turn", out.Rejected)
	}
}

func TestConsumerRegistrationDoesNotClaimProviderRoute(t *testing.T) {
	h := newTestHost("sandbox", "web-tools")
	h.plugins["sandbox"].discovery.Manifest.Consumes = []plugin.ConsumesMethod{{
		Capability: "web", Method: "fetch",
	}}
	h.plugins["web-tools"].discovery.Manifest.Provides = []string{"web"}

	register := func(instance string, id string) {
		t.Helper()
		if err := h.handleRequest(instance, &Frame{
			ID: id, Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
			Payload: mustJSON(map[string]any{"handlers": []map[string]string{{
				"capability": "web", "method": "fetch",
			}}}),
		}); err != nil {
			t.Fatalf("register %s: %v", instance, err)
		}
	}

	register("sandbox", "sandbox-reg")
	if owner := h.routeOwner("web", "fetch"); owner != "" {
		t.Fatalf("consumer claimed route owner %q", owner)
	}
	register("web-tools", "web-reg")
	if owner := h.routeOwner("web", "fetch"); owner != "web-tools" {
		t.Fatalf("route owner = %q, want web-tools", owner)
	}
}

func TestInactiveDeclaredConsumerFailsClosed(t *testing.T) {
	h := newTestHost("provider")
	h.plugins["provider"].discovery.Manifest.Provides = []string{"web"}
	h.SetDiscoveries(plugin.Discoveries{Plugins: []plugin.Discovery{
		{Dir: "provider", Manifest: plugin.Manifest{Name: "provider", Provides: []string{"web"}}},
		{Dir: "sandbox", Manifest: plugin.Manifest{Name: "sandbox", Consumes: []plugin.ConsumesMethod{{
			Capability: "web", Method: "fetch",
		}}}},
	}})
	if err := h.handleRequest("provider", &Frame{
		ID: "reg", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
		Payload: mustJSON(map[string]any{"handlers": []map[string]string{{
			"capability": "web", "method": "fetch",
		}}}),
	}); err != nil {
		t.Fatal(err)
	}
	err := h.handleRequest("provider", &Frame{
		ID: "call", Type: FrameRequest, Capability: "web", Method: "fetch",
		Payload: json.RawMessage(`{}`),
	})
	if !errors.Is(err, ErrRouteFailed) {
		t.Fatalf("error = %v, want route_failed", err)
	}
}

func TestHostRegisterSamePluginLastWins(t *testing.T) {
	h := newTestHost("a")
	req := &Frame{
		ID: "a-1", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
		Payload: mustJSON(map[string]any{"handlers": []map[string]string{
			{"capability": "loop", "method": "turn"},
		}}),
	}
	if err := h.handleRequest("a", req); err != nil {
		t.Fatalf("register: %v", err)
	}
	req2 := &Frame{
		ID: "a-2", Type: FrameRequest, Capability: HostCapability, Method: HostMethodRegister,
		Payload: mustJSON(map[string]any{"handlers": []map[string]string{
			{"capability": "loop", "method": "turn"},
		}}),
	}
	if err := h.handleRequest("a", req2); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if got := h.routeOwner("loop", "turn"); got != "a" {
		t.Fatalf("routeOwner = %q, want a", got)
	}
	frames := drainFrames(t, h, "a")
	if len(frames) != 2 {
		t.Fatalf("got %d res, want 2", len(frames))
	}
	for _, f := range frames {
		if f.ErrorCode != "" {
			t.Fatalf("res has error: %s %s", f.ErrorCode, f.ErrorMsg)
		}
	}
}

func TestForwardInjectsFromAndRestoresOnResponse(t *testing.T) {
	h := newTestHost("a", "b")
	h.routes["llm.complete"] = "b"

	req := &Frame{ID: "agent-3", Type: FrameRequest, Capability: "llm", Method: "complete",
		Payload: mustJSON(map[string]string{"x": "y"})}
	if err := h.handleRequest("a", req); err != nil {
		t.Fatalf("forward: %v", err)
	}

	// b 收到的 req：id=fwd-1、from=a
	bf := drainFrames(t, h, "b")
	if len(bf) != 1 {
		t.Fatalf("b got %d frames, want 1", len(bf))
	}
	fwd := bf[0]
	if fwd.ID != "fwd-1" || fwd.From != "a" {
		t.Fatalf("fwd frame = id:%s from:%s, want fwd-1/a", fwd.ID, fwd.From)
	}

	// b 回 res（fwd-1）→ a 收到 id=agent-3、from=b
	res := &Frame{ID: "fwd-1", Type: FrameResponse, Capability: "llm", Method: "complete",
		Payload: mustJSON(map[string]string{"ok": "1"})}
	if err := h.handleResponse(res); err != nil {
		t.Fatalf("response: %v", err)
	}
	af := drainFrames(t, h, "a")
	if len(af) != 1 {
		t.Fatalf("a got %d frames, want 1", len(af))
	}
	back := af[0]
	if back.ID != "agent-3" || back.From != "b" || back.Type != FrameResponse {
		t.Fatalf("res to a = id:%s from:%s type:%s", back.ID, back.From, back.Type)
	}
	if len(h.pending) != 0 {
		t.Fatalf("pending not drained: %v", h.pending)
	}
}

func TestAttributedEventForwardRestoresID(t *testing.T) {
	h := newTestHost("a", "b")
	h.routes["llm.complete"] = "b"

	req := &Frame{ID: "agent-3", Type: FrameRequest, Capability: "llm", Method: "complete"}
	if err := h.handleRequest("a", req); err != nil {
		t.Fatalf("forward: %v", err)
	}

	// b 发归因 evt（id=fwd-1）→ a 收到 id=agent-3、from=b
	ev := &Frame{ID: "fwd-1", Type: FrameEvent, Capability: "llm", Method: "chunk",
		Payload: mustJSON(map[string]string{"channel": "content", "delta": "hi"})}
	if err := h.handleEvent("b", ev); err != nil {
		t.Fatalf("event: %v", err)
	}
	af := drainFrames(t, h, "a")
	if len(af) != 1 {
		t.Fatalf("a got %d frames, want 1", len(af))
	}
	got := af[0]
	if got.ID != "agent-3" || got.From != "b" || got.Type != FrameEvent {
		t.Fatalf("evt to a = id:%s from:%s type:%s", got.ID, got.From, got.Type)
	}

	// res 仍能闭环
	res := &Frame{ID: "fwd-1", Type: FrameResponse, Capability: "llm", Method: "complete"}
	if err := h.handleResponse(res); err != nil {
		t.Fatalf("response: %v", err)
	}
}

func TestWaitHostResponseGoesToResCh(t *testing.T) {
	h := newTestHost("b")
	resCh := make(chan *Frame, 1)
	h.pending["fwd-9"] = &wait{
		kind: WaitHost, caller: FromHost, target: "b", frameID: "host-1",
		capability: "llm", method: "complete", resCh: resCh,
	}

	res := &Frame{ID: "fwd-9", Type: FrameResponse, Capability: "llm", Method: "complete",
		Payload: mustJSON(map[string]string{"r": "1"})}
	if err := h.handleResponse(res); err != nil {
		t.Fatalf("response: %v", err)
	}
	select {
	case got := <-resCh:
		if got.ID != "host-1" || got.From != "b" {
			t.Fatalf("resCh got id:%s from:%s, want host-1/b", got.ID, got.From)
		}
	default:
		t.Fatal("resCh empty, want res delivered")
	}
	// 不应写回任何插件
	if frames := drainFrames(t, h, "b"); len(frames) != 0 {
		t.Fatalf("b got %d frames, want 0", len(frames))
	}
}

func TestWaitHostEventGoesToOnEvent(t *testing.T) {
	h := newTestHost("b")
	var got *Frame
	h.pending["fwd-9"] = &wait{
		kind: WaitHost, caller: FromHost, target: "b", frameID: "host-1",
		capability: "loop", method: "turn",
		onEvent: func(f *Frame) { got = f },
	}

	ev := &Frame{ID: "fwd-9", Type: FrameEvent, Capability: "loop", Method: "chunk",
		Payload: mustJSON(map[string]string{"channel": "content", "delta": "x"})}
	if err := h.handleEvent("b", ev); err != nil {
		t.Fatalf("event: %v", err)
	}
	if got == nil {
		t.Fatal("onEvent not called")
	}
	if got.ID != "host-1" || got.From != "b" {
		t.Fatalf("onEvent got id:%s from:%s, want host-1/b", got.ID, got.From)
	}
}

func TestUnknownMethodSynthesizesError(t *testing.T) {
	h := newTestHost("a")
	req := &Frame{ID: "a-1", Type: FrameRequest, Capability: "ghost", Method: "x"}
	if err := h.handleRequest("a", req); err == nil {
		t.Fatal("want error for unknown method")
	}
	af := drainFrames(t, h, "a")
	if len(af) != 1 {
		t.Fatalf("a got %d frames, want 1", len(af))
	}
	if af[0].ErrorCode != CodeMethodNotFound || af[0].From != FromHost {
		t.Fatalf("res = code:%s from:%s", af[0].ErrorCode, af[0].From)
	}
}

func TestBroadcastEventDropped(t *testing.T) {
	h := newTestHost("a")
	var logged []string
	h.hlog = func(m string) { logged = append(logged, m) }

	ev := &Frame{ID: "", Type: FrameEvent, Capability: "presentation", Method: "stream"}
	if err := h.handleEvent("a", ev); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if frames := drainFrames(t, h, "a"); len(frames) != 0 {
		t.Fatalf("a got %d frames, want 0", len(frames))
	}
	if len(logged) == 0 || !strings.Contains(logged[0], "broadcast") {
		t.Fatalf("logs = %v, want broadcast drop log", logged)
	}
}

func TestRouteOwnerDoesNotUseProvidesFallback(t *testing.T) {
	h := newTestHost("b")
	h.discoveries = plugin.Discoveries{Plugins: []plugin.Discovery{{
		Manifest: plugin.Manifest{Name: "b", Provides: []string{"llm"}},
	}}}

	if got := h.routeOwner("llm", "complete"); got != "" {
		t.Fatalf("routeOwner before registration = %q, want empty", got)
	}

	h.routes["llm.complete"] = "b"
	if got := h.routeOwner("llm", "complete"); got != "b" {
		t.Fatalf("routeOwner after registration = %q, want b", got)
	}
}
