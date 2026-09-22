package host

import (
	"errors"
	"fmt"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// register.go 注册插件

// register registers the MOUNTED plugins' provides into the host registry.
func (h *Host) register(scans []plugin.Discovery) error {
	var errs []error
	for _, scan := range scans {
		manifest := scan.Manifest
		if err := h.registerProvides(manifest); err != nil {
			errs = append(errs, fmt.Errorf("register plugin %s: %w", manifest.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (h *Host) registerProvides(manifest plugin.Manifest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, provide := range manifest.Provides {
		// tools 允许多属主：只登记第一个，路由不查这张表。
		if provide == ToolsCapability {
			if _, ok := h.provides[provide]; !ok {
				h.provides[provide] = manifest.Name
			}
			continue
		}
		if owner, ok := h.provides[provide]; ok && owner != manifest.Name {
			return fmt.Errorf(
				"plugin %s wants to provide %s, already provided by %s: %w",
				manifest.Name, provide, owner, ErrCapabilityConflict,
			)
		}
		h.provides[provide] = manifest.Name
	}
	return nil
}
