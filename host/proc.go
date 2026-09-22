package host

import (
	"os/exec"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// launch the plugin by add a proc
func (h *Host) launch(discovery plugin.Discovery) error {
	manifest := discovery.Manifest
	if h.IsPluginDisabled(manifest.Name) {
		return ErrPluginDisabled
	}
	entry := manifest.Entry
	//ext := filepath.Ext(entry)
	//if utils.IsLinux() {
	//	entry = filepath.
	//}
	cmd := exec.Command(entry)
	// bind std
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

}

func (h *Host) markUnhealthy(name string) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	p := h.plugins[name]
	if p == nil || p.gen != gen {
		h.mu.Unlock()
		return
	}
	p.healthy = false
}
