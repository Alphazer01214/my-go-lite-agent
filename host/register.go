package host

import (
	"fmt"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// register.go 注册插件

// register registers the MOUNTED plugins
func (h *Host) register(scans []plugin.Discovery) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for _, scan := range scans {
		manifest := scan.Manifest
		if err := h.registerProvides(manifest); err != nil {
			errs = append(errs, fmt.Errorf("can't register plugin %v: %w", manifest.Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("register plugins failed: %w", errs)
	}
	return nil
}

func (h *Host) registerProvides(manifest plugin.Manifest) error {
	for _, provide := range manifest.Provides {
		h.mu.Lock()
		if plg, ok := h.provides[provide]; ok && plg != manifest.Name {
			return fmt.Errorf("plugin %v wants to register capability %v, but it is already provided by %v", manifest.Name, provide, plg)
		}
		h.provides[provide] = manifest.Name
		h.mu.Unlock()
	}
	return nil
}
