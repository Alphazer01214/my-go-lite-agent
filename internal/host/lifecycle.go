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
	if current != nil && current.state == ProviderStateActive {
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
	// Routes are dropped up front so the replacement process can re-register the
	// same keys. launchInstance only spawns the process; the route table is
	// written later by hostRegister, so a failed start leaves nothing behind.
	h.unregisterPlugin(name)

	if err := h.launchInstance(name, discovery, ""); err != nil {
		return err
	}
	h.hlog(fmt.Sprintf("plugin started plugin=%s", name))
	return nil
}

// StopPlugin stops every process instance belonging to one plugin name.
// New Calls stop immediately; existing Calls get one shutdown grace period.
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

	h.mu.Lock()
	var targets []*proc
	for instanceID, current := range h.plugins {
		if current == nil || (current.pluginName != name && instanceID != name) {
			continue
		}
		current.state = ProviderStateStopping
		targets = append(targets, current)
	}
	h.mu.Unlock()
	if len(targets) == 0 {
		return nil
	}

	instanceIDs := make([]string, 0, len(targets))
	for _, current := range targets {
		instanceIDs = append(instanceIDs, current.instanceID)
	}
	if !h.waitForPendingTargets(instanceIDs, DefaultShutdownGrace) {
		for _, instanceID := range instanceIDs {
			h.failInstancePending(instanceID, CodePluginDown, "plugin stopped before Call completed")
		}
	}

	for _, current := range targets {
		stopProc(current)
		h.markUnhealthy(current.instanceID, current.gen)
		h.mu.Lock()
		if h.plugins[current.instanceID] == current {
			delete(h.plugins, current.instanceID)
		}
		h.mu.Unlock()
	}
	h.unregisterPlugin(name)
	h.hlog(fmt.Sprintf("plugin stopped plugin=%s instances=%d", name, len(targets)))
	return nil
}

func (h *Host) waitForPendingTargets(instanceIDs []string, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		h.mu.RLock()
		remaining := 0
		for _, wt := range h.pending {
			for _, instanceID := range instanceIDs {
				if wt.target == instanceID {
					remaining++
					break
				}
			}
		}
		h.mu.RUnlock()
		if remaining == 0 {
			return true
		}

		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

// removeProcess tears down a stale process identity before a manual restart.
func (h *Host) removeProcess(name string, current *proc) {
	if current == nil {
		return
	}
	h.markUnhealthy(name, current.gen)
	stopProc(current)

	h.mu.Lock()
	if h.plugins[name] == current {
		delete(h.plugins, name)
	}
	h.mu.Unlock()
}

var _ PluginLifecycle = (*Host)(nil)
