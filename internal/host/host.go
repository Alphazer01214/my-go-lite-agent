package host

// host/host.go 处理主要的插件服务逻辑
import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// Host handles the lifecycle/registry/transport of plugins
type Host struct {
	mu sync.RWMutex
	// plugins process map
	// plugin name -> proc
	plugins map[string]*proc
	// pending wait map
	// plugin id -> wait
	// id is the forward id (fwd-xxxxxx) or host call id (host-xxxxxx)
	pending map[string]*wait
	// provides the capability of the host to the plugins
	// register: <provide capability>-<plugin name>
	// must unique
	provides map[string]string
	// disabled plugin name -> true
	disabled map[string]bool
	// discoveries is the result of scanning the pluginDir for binaries
	// including their manifests,
	discoveries plugin.Discoveries
	pluginsDir  string
	// seq
	seq    int
	closed bool
	gen    map[string]int

	mountedUI map[string]bool

	// hlog is the host log CALLBACK function
	hlog func(message string)
}

type proc struct {
	// discovery dir and manifest
	discovery plugin.Discovery
	// cmd is the real process
	cmd *exec.Cmd
	// stdin is the ONLY way to write Frame into the pipeline
	// then read from stdout(always occupied)
	stdin   io.Writer
	gen     int
	mu      sync.Mutex
	healthy bool
	timeout time.Duration
}

type wait struct {
	// caller is the plugin name that called the wait
	caller string
	// target is the plugin name that the caller is waiting for
	target string
	// frameID is the caller's frame id
	frameID string
	// kind
	kind   WaitKind
	events []*Frame

	capability string
	method     string
	payload    json.RawMessage
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

func (h *Host) GetMountedPlugins() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var mounted []string
	for name := range h.plugins {
		mounted = append(mounted, name)
	}
	return mounted
}

func (h *Host) IsPluginDisabled(name string) bool {
	return h.disabled[name]
}

func (h *Host) mountUI() error {
	return nil
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

func Run(discoveries plugin.Discoveries) (*Host, error) {
	h := &Host{
		plugins:     make(map[string]*proc),
		pending:     make(map[string]*wait),
		provides:    make(map[string]string),
		discoveries: discoveries,
		disabled:    make(map[string]bool),
		mountedUI:   make(map[string]bool),
		gen:         make(map[string]int),
		hlog:        defaultLogFunc,
	}

	//plugins := discoveries.Plugins
	// TODO 试图加载所有插件（无论冲突），然后在完毕后列出冲突警告
	// TODO mount plugins
	//for _, p := range plugins {
	//	if err := h.mountPlugin(p); err != nil {
	//		return nil, err
	//	}
	//}
	return h, nil
}

func refresh() {

}

func defaultLogFunc(message string) {
	fmt.Println(message)
}
