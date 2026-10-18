package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type processSpec struct {
	Command        string
	Args           []string
	Dir            string
	Env            []string
	TimeoutMS      int
	MaxOutputBytes int
}

type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	total     int
	truncated bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += len(data)
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		write := len(data)
		if write > remaining {
			write = remaining
		}
		_, _ = b.buf.Write(data[:write])
	}
	if b.total > b.limit {
		b.truncated = true
	}
	return len(data), nil
}

func (b *cappedBuffer) snapshot() (string, int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String(), b.total, b.truncated
}

func runProcess(ctx context.Context, spec processSpec) (json.RawMessage, error) {
	if spec.TimeoutMS <= 0 {
		spec.TimeoutMS = 120000
	}
	if spec.MaxOutputBytes <= 0 {
		spec.MaxOutputBytes = 1 << 20
	}
	started := time.Now()
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	stdout := newCappedBuffer(spec.MaxOutputBytes)
	stderr := newCappedBuffer(spec.MaxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, pluginsdk.ErrCode("command_not_found", err.Error())
		}
		return nil, pluginsdk.ErrCode("process_start_failed", err.Error())
	}
	if err := assignProcess(cmd); err != nil {
		_ = killProcessTree(cmd)
		_ = cmd.Wait()
		return nil, pluginsdk.ErrCode("process_start_failed", err.Error())
	}

	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	timer := time.NewTimer(time.Duration(spec.TimeoutMS) * time.Millisecond)
	defer timer.Stop()
	timedOut := false
	cancelled := false
	var waitErr error
	select {
	case waitErr = <-wait:
	case <-timer.C:
		timedOut = true
		_ = killProcessTree(cmd)
		waitErr = <-wait
	case <-ctx.Done():
		cancelled = true
		_ = killProcessTree(cmd)
		waitErr = <-wait
	}
	releaseProcess(cmd)
	if cancelled {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "call deadline exceeded")
		}
		return nil, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "call cancelled")
	}
	finished := time.Now()
	stdoutText, stdoutBytes, stdoutTruncated := stdout.snapshot()
	stderrText, stderrBytes, stderrTruncated := stderr.snapshot()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if waitErr != nil {
		exitCode = -1
	}
	meta := map[string]any{
		"exit_code":        exitCode,
		"stdout_bytes":     stdoutBytes,
		"stderr_bytes":     stderrBytes,
		"stdout_truncated": stdoutTruncated,
		"stderr_truncated": stderrTruncated,
		"started_at":       started.UTC().Format(time.RFC3339Nano),
		"finished_at":      finished.UTC().Format(time.RFC3339Nano),
		"duration_ms":      finished.Sub(started).Milliseconds(),
		"timed_out":        timedOut,
	}
	content := fmt.Sprintf("exit_code: %d\nstdout:\n%s\nstderr:\n%s", exitCode, stdoutText, stderrText)
	if timedOut {
		meta["code"] = "process_timeout"
	}
	isError := exitCode != 0 || timedOut
	if waitErr != nil && exitCode == 0 && !timedOut {
		meta["process_error"] = waitErr.Error()
		isError = true
	}
	return toolcontract.Encode(toolcontract.Result{
		Content: strings.TrimRight(content, "\n"), IsError: isError,
		Truncated: stdoutTruncated || stderrTruncated, Meta: meta,
	})
}
