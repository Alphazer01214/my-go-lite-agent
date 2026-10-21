//go:build windows

package main

import (
	"os/exec"
	"sync"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

const codepageUTF8 = 65001

// decodeOutputBytes converts console output written in the active ANSI code
// page (GBK on Chinese Windows) to UTF-8. Valid UTF-8 is left untouched, so
// tools that already emit UTF-8 keep their bytes.
func decodeOutputBytes(data []byte) []byte {
	if len(data) == 0 || utf8.Valid(data) {
		return data
	}
	codePage := windows.GetACP()
	if codePage == codepageUTF8 {
		return data
	}
	size, err := windows.MultiByteToWideChar(codePage, 0, &data[0], int32(len(data)), nil, 0)
	if err != nil || size <= 0 {
		return data
	}
	wide := make([]uint16, size)
	if _, err := windows.MultiByteToWideChar(codePage, 0, &data[0], int32(len(data)), &wide[0], size); err != nil {
		return data
	}
	return []byte(string(utf16.Decode(wide)))
}

var jobs sync.Map

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func assignProcess(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		return err
	}
	jobs.Store(cmd.Process.Pid, job)
	return nil
}

func killProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if value, ok := jobs.Load(cmd.Process.Pid); ok {
		return windows.TerminateJobObject(value.(windows.Handle), 1)
	}
	return cmd.Process.Kill()
}

func releaseProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if value, ok := jobs.LoadAndDelete(cmd.Process.Pid); ok {
		_ = windows.CloseHandle(value.(windows.Handle))
	}
}
