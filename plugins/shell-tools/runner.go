package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

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

func runProcess(ctx context.Context, spec processSpec) (toolResult, error) {
	if spec.TimeoutMS <= 0 {
		spec.TimeoutMS = defaultTimeoutMS
	}
	if spec.MaxOutputBytes <= 0 {
		spec.MaxOutputBytes = defaultMaxOutputBytes
	}

	started := time.Now()
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	cmd.WaitDelay = 5 * time.Second
	stdout := newCappedBuffer(spec.MaxOutputBytes)
	stderr := newCappedBuffer(spec.MaxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureProcess(cmd)

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return toolResult{}, pluginsdk.ErrCode(codeCommandNotFound, err.Error())
		}
		return toolResult{}, pluginsdk.ErrCode(codeProcessStartFailed, err.Error())
	}
	if err := assignProcess(cmd); err != nil {
		_ = killProcessTree(cmd)
		_ = cmd.Wait()
		return toolResult{}, pluginsdk.ErrCode(codeProcessStartFailed, err.Error())
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
	if errors.Is(waitErr, exec.ErrWaitDelay) {
		_ = killProcessTree(cmd)
	}
	releaseProcess(cmd)

	if cancelled {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return toolResult{}, pluginsdk.ErrCode(pluginsdk.CodeCallDeadlineExceeded, "call deadline exceeded")
		}
		return toolResult{}, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "call cancelled")
	}

	finished := time.Now()
	stdoutText, stdoutBytes, stdoutTruncated := stdout.snapshot()
	stderrText, stderrBytes, stderrTruncated := stderr.snapshot()
	stdoutText = string(decodeOutputBytes([]byte(stdoutText)))
	stderrText = string(decodeOutputBytes([]byte(stderrText)))

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
	if spec.Stdin != "" {
		meta["stdin_bytes"] = len(spec.Stdin)
	}

	content := fmt.Sprintf("exit_code: %d\nstdout:\n%s\nstderr:\n%s", exitCode, stdoutText, stderrText)
	content, contentTruncated := truncateUTF8(strings.TrimRight(content, "\n"), maxContentBytes)
	truncated := stdoutTruncated || stderrTruncated || contentTruncated
	isError := exitCode != 0 || timedOut
	if timedOut {
		meta["code"] = "process_timeout"
	}
	if waitErr != nil && exitCode == 0 && !timedOut {
		meta["process_error"] = waitErr.Error()
		isError = true
	}

	result := success(content, meta)
	result.IsError = isError
	result.Truncated = truncated
	return result, nil
}

func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}
