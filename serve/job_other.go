//go:build !windows

package serve

import (
	"os/exec"
	"syscall"
)

// jobHolder is a no-op on non-Windows; process groups handle reaping.
type jobHolder struct{}

func newJob() (*jobHolder, error) { return &jobHolder{}, nil }

func (j *jobHolder) assign(cmd *exec.Cmd) error {
	if cmd == nil || cmd.SysProcAttr == nil {
		return nil
	}
	return nil
}

func (j *jobHolder) close() {}

func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
