package host

// host/host.go 处理主要的插件服务逻辑
import (
	"encoding/json"
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
	// plugin name -> wait
	pending map[string]*wait
	// provides the capability of the host to the plugins
	// register: <provide capability>-<plugin name>
	// must unique
	provides map[string]string
	// discoveries is the result of scanning the pluginDir for binaries
	// including their manifests,
	discoveries plugin.Discoveries
	pluginsDir  string
	// seq
	seq int

	mountedUI map[string]bool
}

type proc struct {
	// discovery dir and manifest
	discovery plugin.Discovery
	// cmd is the real process
	cmd *exec.Cmd
	// stdin is the ONLY way to write Frame into the pipeline
	// then read from stdout(always occupied)
	stdin   io.Writer
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
	kind WaitKind

	capability string
	method     string
	payload    json.RawMessage
}

type pluginProc struct {
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

// alive reports whether the named plugin is mounted and healthy.
// nil means alive; otherwise a *FrameError implementing error.
func (h *Host) alive(name string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	plg, ok := h.plugins[name]
	if !ok || plg == nil {
		return Errorf(CodePluginNotMounted, "plugin %s not mounted", name)
	}
	if !plg.healthy {
		return Errorf(CodePluginDown, "plugin %s is not healthy", name)
	}
	return nil
}
