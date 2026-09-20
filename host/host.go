// host/host.go 处理主要的插件服务逻辑
package host

import (
	"sync"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// Host handles the lifecycle/registry/transport of plugins
type Host struct {
	mu sync.RWMutex
	// plugins process
	plugins map[string]string
	// discoveries is the result of scanning the pluginDir for binaries
	// including their manifests,
	discoveries []plugin.ScanResult
	pluginDir   string
}
