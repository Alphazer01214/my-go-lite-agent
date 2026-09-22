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
	// stderr, err := cmd.StderrPipe()
	// if err != nil {
	// 	return err
	// }
	if err := cmd.Start(); err != nil {
		return err
	}
	h.mu.Lock()
	h.gen[manifest.Name]++
	gen := h.gen[manifest.Name]
	h.plugins[manifest.Name] = &proc{
		discovery: discovery,
		cmd:       cmd,
		stdin:     stdin,
		gen:       gen,
		healthy:   true,
	}
	h.mu.Unlock()

	go h.read(manifest.Name, gen, stdout)
	return nil

}

func (h *Host) markUnhealthy(name string, gen int) {
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
	h.mu.Unlock()
}
