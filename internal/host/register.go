package host

// unregisterPlugin removes stale discovery-owned routes so a stopped or
// replaced plugin can be started again without route conflicts.
func (h *Host) unregisterPlugin(pluginName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, owner := range h.routes {
		if owner == pluginName {
			delete(h.routes, key)
		}
	}
}
