//go:build !windows

package serve

import (
	"os/exec"
	"syscall"
)

// jobHolder is a no-op on non-Windows: there is no Job Object, so orphaned
// plugin processes are reaped via killTree when the Host shuts down.
type jobHolder struct{}

func newJob() (*jobHolder, error) { return &jobHolder{}, nil }

func (j *jobHolder) assign(*exec.Cmd) error { return nil }

func (j *jobHolder) close() {}

func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
