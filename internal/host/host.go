package host

// host/host.go 处理主要的插件服务逻辑
import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// Host handles the lifecycle/registry/transport of plugins
type Host struct {
	mu sync.RWMutex
	// plugins process map
	// instance id -> proc
	plugins map[string]*proc
	// pending wait map
	// plugin id -> wait
	// id is the forward id (fwd-xxxxxx) or host call id (host-xxxxxx)
	pending map[string]*wait
	// provides the capability of the host to the plugins
	// register: <provide capability>-<plugin name>
	// must unique
	provides map[string]string
	// routes maps "capability.method" -> owning plugin name (host.register 上报)
	// 查找优先于 provides（register 上报异步，provides 回落兜启动竞态）
	routes map[string]string
	// disabled plugin name -> true
	disabled map[string]bool
	// stopped records explicit manual stop intent for the current Host run.
	stopped map[string]bool
	// startupPlanned records the initial autostart closure, including failed launches.
	startupPlanned map[string]bool
	// lifecycleMu serializes rescan/start/stop transitions.
	lifecycleMu sync.Mutex
	// discoveries is the result of scanning the pluginDir for binaries
	// including their manifests,
	discoveries plugin.Discoveries
	pluginsDir  string
	// seq
	seq    int
	closed bool
	gen    map[string]int

	// contexts is Host's generic scheduling scope registry.
	contexts   map[string]*ExecutionContext
	contextSeq int

	mountedUI map[string]bool

	// hlog is the host log CALLBACK function
	hlog func(message string)
	// trace observes every Frame crossing the Host boundary (debug; nil = off)
	uiPublisher UIPublisher
	trace       TraceFunc
}

type proc struct {
	// discovery dir and manifest
	discovery plugin.Discovery
	// instanceID identifies this process activation; pluginName remains Frame identity.
	instanceID     string
	pluginName     string
	configRevision string
	// cmd is the real process
	cmd *exec.Cmd
	// stdin is the ONLY way to write Frame into the pipeline
	// then read from stdout(always occupied)
	stdin io.WriteCloser
	gen   int
	mu    sync.Mutex
	// done is closed after cmd.Wait() returns (process fully reaped)
	done     chan struct{}
	healthy  bool
	stopping bool
	timeout  time.Duration
}

type wait struct {
	// caller is the source instance id for plugin-originated calls.
	caller       string
	callerPlugin string
	// target is the resolved owner instance id.
	target       string
	targetPlugin string
	contextID    string
	// frameID is the caller's frame id
	frameID string
	// kind
	kind   WaitKind
	events []*Frame

	capability string
	method     string
	payload    json.RawMessage

	// resCh / onEvent serve WaitHost (host-initiated calls).
	// resCh is buffered 1; death closure and handleResponse write exactly one res into it.
	resCh   chan *Frame
	onEvent func(*Frame)
}

func (h *Host) SetPluginDir(dir string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pluginsDir = dir
}

func (h *Host) SetDiscoveries(discoveries plugin.Discoveries) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.discoveries = discoveries
}

func (h *Host) SetLogFunc(logFunc func(message string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hlog = logFunc
}

// TraceFunc 观察 Host 边界上的每条 Frame（调试用）。dir 为 "in"（插件→Host）
// 或 "out"（Host→插件）；plugin 为对端插件名。回调在锁外调用，须快且非阻塞。
type TraceFunc func(dir string, plugin string, frame *Frame)

// SetUIPublisher sets the WebUI state/command publisher.
func (h *Host) SetUIPublisher(publisher UIPublisher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.uiPublisher = publisher
}

// SetTraceFunc 设置 Frame trace 回调；nil 关闭。
func (h *Host) SetTraceFunc(fn TraceFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.trace = fn
}

// tracef 在锁外调用 trace 回调（避免回调重入 Host 造成死锁）。
func (h *Host) tracef(dir string, plugin string, frame *Frame) {
	h.mu.RLock()
	fn := h.trace
	h.mu.RUnlock()
	if fn != nil {
		fn(dir, plugin, frame)
	}
}

func (h *Host) GetMountedPlugins() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var mounted []string
	seen := make(map[string]struct{}, len(h.plugins))
	for instanceID, p := range h.plugins {
		name := instanceID
		if p != nil && p.pluginName != "" {
			name = p.pluginName
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		mounted = append(mounted, name)
	}
	return mounted
}

func (h *Host) IsPluginDisabled(name string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.disabled[name]
}

// alive reports whether the named plugin is mounted and healthy.
// nil means alive; otherwise an error (sentinel via errors.Is).
func (h *Host) alive(name string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	plg, ok := h.plugins[name]
	if !ok || plg == nil {
		return fmt.Errorf("plugin %s not mounted: %w", name, ErrPluginNotMounted)
	}
	if !plg.healthy {
		return fmt.Errorf("plugin %s is not healthy: %w", name, ErrPluginDown)
	}
	return nil
}

// Run mounts the autostart+depends_on closure of discoveries.
// 挂载软失败：单个插件注册冲突或启动失败只记日志并跳过，Host 不退出（protocol §6）。
// logFunc 为 nil 时日志走 stderr（stdout 留给对话）。
func Run(discoveries plugin.Discoveries, logFunc func(string)) (*Host, error) {
	h := &Host{
		plugins:        make(map[string]*proc),
		pending:        make(map[string]*wait),
		provides:       make(map[string]string),
		routes:         make(map[string]string),
		discoveries:    discoveries,
		disabled:       make(map[string]bool),
		stopped:        make(map[string]bool),
		startupPlanned: make(map[string]bool),
		mountedUI:      make(map[string]bool),
		gen:            make(map[string]int),
		contexts:       make(map[string]*ExecutionContext),
		hlog:           defaultLogFunc,
	}
	if logFunc != nil {
		h.hlog = logFunc
	}

	ordered, warns := plugin.AutostartClosure(discoveries)
	for _, d := range ordered {
		h.startupPlanned[d.Manifest.Name] = true
	}
	for _, w := range warns {
		h.hlog("mount warn: " + w)
	}
	// provides 先入表再启动：进程一起来就能路由到已声明能力
	if err := h.register(ordered); err != nil {
		h.hlog(fmt.Sprintf("mount register: %v", err))
	}
	for _, d := range ordered {
		if err := h.launchInstance(d.Manifest.Name, d, ""); err != nil {
			h.hlog(fmt.Sprintf("mount %s: %v", d.Manifest.Name, err))
			continue
		}
	}
	return h, nil
}

// Close shuts the host down: marks closed, closes every plugin stdin (EOF makes
// the SDK Serve loop exit), synthesizes server_closed for remaining pending,
// then kills whatever survives DefaultShutdownGrace.
func (h *Host) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	// 一次性摘走全部 pending，锁外闭环
	waits := make([]*wait, 0, len(h.pending))
	for id, wt := range h.pending {
		waits = append(waits, wt)
		delete(h.pending, id)
	}
	procs := make([]*proc, 0, len(h.plugins))
	for _, p := range h.plugins {
		procs = append(procs, p)
	}
	h.mu.Unlock()

	for _, wt := range waits {
		if wt.kind == WaitHost && wt.resCh != nil {
			select {
			case wt.resCh <- &Frame{
				Version:    FrameVersion,
				ID:         wt.frameID,
				Type:       FrameResponse,
				Capability: wt.capability,
				Method:     wt.method,
				From:       FromHost,
				ErrorCode:  CodeHostServerClosed,
				ErrorMsg:   "host closed",
			}:
			default:
			}
			continue
		}
		h.replyError(wt.caller, wt.frameID, wt.capability, wt.method,
			CodeHostServerClosed, "host closed")
	}

	// stdin EOF → 插件 SDK Listen 退出；回程 write 因 closed 被 alive() 拒绝，不再投递
	for _, p := range procs {
		_ = p.stdin.Close()
	}

	// grace 后强杀未退出的进程（Windows 下不 Kill 会残留）
	deadline := time.NewTimer(DefaultShutdownGrace)
	defer deadline.Stop()
	for _, p := range procs {
		select {
		case <-p.done:
		case <-deadline.C:
			if p.cmd != nil && p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
		}
	}
}

func refresh() {

}

// defaultLogFunc logs to stderr: stdout is reserved for the conversation surface.
func defaultLogFunc(message string) {
	fmt.Fprintln(os.Stderr, message)
}
