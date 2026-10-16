package host

import (
	"fmt"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// PluginLifecycle is the narrow management seam used by the WebUI server.
type PluginLifecycle interface {
	PluginSnapshot() []PluginSnapshot
	RescanPlugins() (plugin.Discoveries, error)
	StartPlugin(name string) error
	StopPlugin(name string) error
}

func (h *Host) discoveryFor(name string) (plugin.Discovery, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, discovery := range h.discoveries.Plugins {
		if discovery.Manifest.Name == name {
			return discovery, true
		}
	}
	return plugin.Discovery{}, false
}

// RescanPlugins performs the only runtime plugin-directory search. It updates
// discovery metadata but never starts newly found plugins automatically.
func (h *Host) RescanPlugins() (plugin.Discoveries, error) {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()

	h.mu.RLock()
	dir := h.pluginsDir
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		return plugin.Discoveries{}, ErrHostClosed
	}
	if dir == "" {
		return plugin.Discoveries{}, fmt.Errorf("plugin directory is not configured: %w", ErrBadArguments)
	}

	discoveries := plugin.ScanRoot(dir)
	h.SetDiscoveries(discoveries)
	h.hlog(fmt.Sprintf("plugins rescanned dir=%s plugins=%d diagnostics=%d",
		dir, len(discoveries.Plugins), len(discoveries.Diagnostics)))
	return discoveries, nil
}

// StartPlugin starts one discovered plugin. Dependencies must already be healthy.
// The operation is idempotent for an already healthy plugin.
func (h *Host) StartPlugin(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name is required: %w", ErrBadArguments)
	}
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()

	if h.isClosed() {
		return ErrHostClosed
	}
	discovery, ok := h.discoveryFor(name)
	if !ok {
		return fmt.Errorf("plugin %s is not discovered: %w", name, ErrPluginNotMounted)
	}

	h.mu.RLock()
	current := h.plugins[name]
	h.mu.RUnlock()
	if current != nil && current.healthy {
		return nil
	}

	for _, dependency := range discovery.Manifest.DependsOn {
		if err := h.alive(dependency); err != nil {
			return fmt.Errorf("plugin %s dependency %s: %w", name, dependency, err)
		}
	}

	if current != nil {
		h.removeProcess(name, current)
	}
	h.unregisterPlugin(name)
	if err := h.registerProvides(discovery.Manifest); err != nil {
		return err
	}

	h.mu.Lock()
	if h.stopped == nil {
		h.stopped = make(map[string]bool)
	}
	h.stopped[name] = false
	h.mu.Unlock()
	if err := h.launchInstance(name, discovery, ""); err != nil {
		h.unregisterPlugin(name)
		return err
	}
	h.hlog(fmt.Sprintf("plugin started plugin=%s", name))
	return nil
}

// StopPlugin stops one plugin process. The manual stop state is retained until
// StartPlugin is called; it is not persisted across Host restarts.
func (h *Host) StopPlugin(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name is required: %w", ErrBadArguments)
	}
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()

	if h.isClosed() {
		return ErrHostClosed
	}
	if _, ok := h.discoveryFor(name); !ok {
		return fmt.Errorf("plugin %s is not discovered: %w", name, ErrPluginNotMounted)
	}

	h.mu.RLock()
	current := h.plugins[name]
	h.mu.RUnlock()
	if current == nil {
		h.mu.Lock()
		if h.stopped == nil {
			h.stopped = make(map[string]bool)
		}
		h.stopped[name] = true
		h.mu.Unlock()
		return nil
	}

	// Close pending work before removing the process identity.
	h.markUnhealthy(name, current.gen)
	_ = current.stdin.Close()

	select {
	case <-current.done:
	case <-time.After(DefaultShutdownGrace):
		if current.cmd != nil && current.cmd.Process != nil {
			_ = current.cmd.Process.Kill()
		}
		select {
		case <-current.done:
		case <-time.After(DefaultShutdownGrace):
		}
	}

	h.mu.Lock()
	if h.plugins[name] == current {
		delete(h.plugins, name)
	}
	if h.stopped == nil {
		h.stopped = make(map[string]bool)
	}
	h.stopped[name] = true
	h.mu.Unlock()
	h.unregisterPlugin(name)
	h.hlog(fmt.Sprintf("plugin stopped plugin=%s", name))
	return nil
}

// removeProcess tears down a stale process identity before a manual restart.
func (h *Host) removeProcess(name string, current *proc) {
	if current == nil {
		return
	}
	h.markUnhealthy(name, current.gen)
	_ = current.stdin.Close()
	select {
	case <-current.done:
	case <-time.After(DefaultShutdownGrace):
		if current.cmd != nil && current.cmd.Process != nil {
			_ = current.cmd.Process.Kill()
		}
		select {
		case <-current.done:
		case <-time.After(DefaultShutdownGrace):
		}
	}
	h.mu.Lock()
	if h.plugins[name] == current {
		delete(h.plugins, name)
	}
	h.mu.Unlock()
}

// IsPluginStopped reports explicit manual stop intent.
func (h *Host) IsPluginStopped(name string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.stopped[name]
}

var _ PluginLifecycle = (*Host)(nil)
