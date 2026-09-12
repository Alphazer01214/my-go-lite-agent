//go:build windows

package serve

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobHolder tracks a Windows Job Object that kills children when the Host dies.
type jobHolder struct {
	handle windows.Handle
}

func newJob() (*jobHolder, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &jobHolder{handle: h}, nil
}

func (j *jobHolder) assign(cmd *exec.Cmd) error {
	if j == nil || cmd == nil || cmd.Process == nil {
		return nil
	}
	// PROCESS_ALL_ACCESS is broader than needed but reliable across Windows versions.
	// Prefer the minimal set when available.
	const processSetQuota = 0x0100
	const processTerminate = 0x0001
	h, err := windows.OpenProcess(processSetQuota|processTerminate, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return windows.AssignProcessToJobObject(j.handle, h)
}

func (j *jobHolder) close() {
	if j == nil || j.handle == 0 {
		return
	}
	// Closing the job with KILL_ON_JOB_CLOSE terminates remaining children.
	_ = windows.CloseHandle(j.handle)
	j.handle = 0
}

// killTree force-kills a process and its children (fallback when Job Object missed).
func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	// taskkill /T kills the tree on Windows.
	kill := exec.Command("taskkill", "/T", "/F", "/PID", itoa(pid))
	const createNoWindow = 0x08000000
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	_ = kill.Run()
	_ = cmd.Process.Kill()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
