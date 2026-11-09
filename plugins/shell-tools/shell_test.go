package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SHELL_HELPER") != "1" {
		return
	}
	args := os.Args
	for index, arg := range args {
		if arg == "--" {
			args = args[index+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	switch args[0] {
	case "echo":
		fmt.Fprint(os.Stdout, args[1])
	case "stderr":
		fmt.Fprint(os.Stderr, args[1])
	case "exit":
		code, _ := strconv.Atoi(args[1])
		os.Exit(code)
	case "env":
		fmt.Fprint(os.Stdout, os.Getenv(args[1]))
	case "cwd":
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprint(os.Stdout, dir)
	case "stdin":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(2)
		}
		_, _ = os.Stdout.Write(data)
	case "sleep":
		milliseconds, _ := strconv.Atoi(args[1])
		time.Sleep(time.Duration(milliseconds) * time.Millisecond)
	case "big":
		size, _ := strconv.Atoi(args[1])
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("y"), size))
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func helperCommand() string {
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		return os.Args[0]
	}
	return path
}

func helperArgs(args ...string) []string {
	return append([]string{"-test.run=TestHelperProcess", "--"}, args...)
}

func helperSpec(t *testing.T, args ...string) processSpec {
	t.Helper()
	env, err := mergedEnv(map[string]string{"GO_WANT_SHELL_HELPER": "1"})
	if err != nil {
		t.Fatalf("mergedEnv: %v", err)
	}
	return processSpec{
		Command: helperCommand(),
		Args:    helperArgs(args...),
		Dir:     t.TempDir(),
		Env:     env,
	}
}

func decodeMeta(t *testing.T, result toolResult) map[string]any {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal(result.Meta, &meta); err != nil {
		t.Fatalf("meta: %v (%s)", err, result.Meta)
	}
	return meta
}

func TestRunProcessSuccess(t *testing.T) {
	spec := helperSpec(t, "echo", "hello")
	result, err := runProcess(context.Background(), spec)
	if err != nil {
		t.Fatalf("runProcess: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %+v", result)
	}
	if !strings.Contains(result.Content, "hello") {
		t.Fatalf("content = %q", result.Content)
	}
	meta := decodeMeta(t, result)
	if meta["exit_code"] != float64(0) {
		t.Fatalf("exit_code = %v", meta["exit_code"])
	}
	if meta["stdout_bytes"] != float64(5) {
		t.Fatalf("stdout_bytes = %v", meta["stdout_bytes"])
	}
}

func TestRunProcessNonZeroExit(t *testing.T) {
	spec := helperSpec(t, "exit", "3")
	result, err := runProcess(context.Background(), spec)
	if err != nil {
		t.Fatalf("runProcess: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result: %+v", result)
	}
	if !strings.Contains(result.Content, "exit_code: 3") {
		t.Fatalf("content = %q", result.Content)
	}
	if decodeMeta(t, result)["exit_code"] != float64(3) {
		t.Fatalf("meta = %s", result.Meta)
	}
}

func TestRunProcessStdin(t *testing.T) {
	spec := helperSpec(t, "stdin")
	spec.Stdin = "hello stdin"
	result, err := runProcess(context.Background(), spec)
	if err != nil {
		t.Fatalf("runProcess: %v", err)
	}
	if !strings.Contains(result.Content, "hello stdin") {
		t.Fatalf("content = %q", result.Content)
	}
	if decodeMeta(t, result)["stdin_bytes"] != float64(len("hello stdin")) {
		t.Fatalf("meta = %s", result.Meta)
	}
}

func TestRunProcessTruncation(t *testing.T) {
	spec := helperSpec(t, "big", "4096")
	spec.MaxOutputBytes = 128
	result, err := runProcess(context.Background(), spec)
	if err != nil {
		t.Fatalf("runProcess: %v", err)
	}
	if !result.Truncated {
		t.Fatalf("expected truncated result: %+v", result)
	}
	meta := decodeMeta(t, result)
	if meta["stdout_bytes"] != float64(4096) || meta["stdout_truncated"] != true {
		t.Fatalf("meta = %s", result.Meta)
	}
	if strings.Count(result.Content, "y") != 128 {
		t.Fatalf("stdout y count = %d", strings.Count(result.Content, "y"))
	}
}

func TestRunProcessTimeout(t *testing.T) {
	spec := helperSpec(t, "sleep", "5000")
	spec.TimeoutMS = 50
	result, err := runProcess(context.Background(), spec)
	if err != nil {
		t.Fatalf("runProcess: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result: %+v", result)
	}
	meta := decodeMeta(t, result)
	if meta["timed_out"] != true || meta["code"] != "process_timeout" {
		t.Fatalf("meta = %s", result.Meta)
	}
}

func TestRunProcessCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	spec := helperSpec(t, "sleep", "5000")
	_, err := runProcess(ctx, spec)
	if pluginsdk.Code(err) != pluginsdk.CodeCallCancelled {
		t.Fatalf("err = %v", err)
	}
}

func TestRunProcessCommandNotFound(t *testing.T) {
	spec := processSpec{
		Command: filepath.Join(t.TempDir(), "missing-command"),
		Dir:     t.TempDir(),
		Env:     nil,
	}
	_, err := runProcess(context.Background(), spec)
	if pluginsdk.Code(err) != codeCommandNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestExecuteResolvesCwdAndEnv(t *testing.T) {
	workspace := t.TempDir()
	sub := filepath.Join(workspace, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := &plugin{}
	result, err := p.execute(context.Background(), &providerRequest{Workspace: workspace}, &executeArgs{
		Command: helperCommand(),
		Args:    helperArgs("cwd"),
		Cwd:     "sub",
		Env:     map[string]string{"GO_WANT_SHELL_HELPER": "1"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result.Content, sub) {
		t.Fatalf("content = %q, want cwd %q", result.Content, sub)
	}

	outside := t.TempDir()
	result, err = p.execute(context.Background(), &providerRequest{Workspace: workspace}, &executeArgs{
		Command: helperCommand(),
		Args:    helperArgs("cwd"),
		Cwd:     outside,
		Env:     map[string]string{"GO_WANT_SHELL_HELPER": "1"},
	})
	if err != nil {
		t.Fatalf("execute outside cwd: %v", err)
	}
	if !strings.Contains(result.Content, outside) {
		t.Fatalf("content = %q, want cwd %q", result.Content, outside)
	}

	result, err = p.execute(context.Background(), &providerRequest{Workspace: workspace}, &executeArgs{
		Command: helperCommand(),
		Args:    helperArgs("env", "LITEAGENT_TEST_ENV"),
		Env: map[string]string{
			"GO_WANT_SHELL_HELPER": "1",
			"LITEAGENT_TEST_ENV":   "works",
		},
	})
	if err != nil {
		t.Fatalf("execute env: %v", err)
	}
	if !strings.Contains(result.Content, "works") {
		t.Fatalf("content = %q", result.Content)
	}
}

func TestExecuteValidation(t *testing.T) {
	p := &plugin{}
	ctx := context.Background()

	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Command: " "}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("empty command err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Command: "go", Args: []string{"bad\x00arg"}}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("NUL arg err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Command: "go", Env: map[string]string{"BAD-NAME": "x"}}); pluginsdk.Code(err) != codeEnvInvalid {
		t.Fatalf("invalid env err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Command: "go", Cwd: "missing-dir"}); pluginsdk.Code(err) != codeCwdInvalid {
		t.Fatalf("missing cwd err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Command: "go", TimeoutMS: -1}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("negative timeout err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{}, &executeArgs{Command: "go", MaxOutputBytes: -1}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("negative max output err = %v", err)
	}
	if _, err := p.execute(ctx, &providerRequest{Workspace: t.TempDir()}, &executeArgs{Command: `python -c "print(1)"`}); pluginsdk.Code(err) != pluginsdk.CodeBadArguments {
		t.Fatalf("command with args err = %v", err)
	} else if !strings.Contains(err.Error(), "args[]") {
		t.Fatalf("command hint = %v, want args[] guidance", err)
	}
}

func TestMergedEnv(t *testing.T) {
	t.Setenv("LITEAGENT_INHERITED", "yes")
	env, err := mergedEnv(map[string]string{"LITEAGENT_OVERRIDE": "value"})
	if err != nil {
		t.Fatalf("mergedEnv: %v", err)
	}
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "LITEAGENT_INHERITED=yes") || !strings.Contains(joined, "LITEAGENT_OVERRIDE=value") {
		t.Fatalf("env = %v", env)
	}
	if _, err := mergedEnv(map[string]string{"BAD-NAME": "x"}); pluginsdk.Code(err) != codeEnvInvalid {
		t.Fatalf("invalid name err = %v", err)
	}
	if _, err := mergedEnv(map[string]string{"GOOD": "bad\x00value"}); pluginsdk.Code(err) != codeEnvInvalid {
		t.Fatalf("invalid value err = %v", err)
	}
}

func TestResolveCwd(t *testing.T) {
	workspace := t.TempDir()
	sub := filepath.Join(workspace, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got, err := resolveCwd(workspace, "sub"); err != nil || got != sub {
		t.Fatalf("resolveCwd sub = %q, %v", got, err)
	}
	if got, err := resolveCwd(workspace, sub); err != nil || got != sub {
		t.Fatalf("resolveCwd absolute = %q, %v", got, err)
	}
	if _, err := resolveCwd(workspace, "missing"); pluginsdk.Code(err) != codeCwdInvalid {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := resolveCwd(workspace, "bad\x00path"); pluginsdk.Code(err) != codeCwdInvalid {
		t.Fatalf("NUL err = %v", err)
	}
}

func TestResolveCommand(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "work")
	if got := resolveCommand("go", cwd); got != "go" {
		t.Fatalf("bare command = %q", got)
	}
	absolute := filepath.Join(cwd, "tool")
	if got := resolveCommand(absolute, cwd); got != absolute {
		t.Fatalf("absolute command = %q", got)
	}
	relative := "." + string(filepath.Separator) + "tool"
	if got := resolveCommand(relative, cwd); got != filepath.Join(cwd, relative) {
		t.Fatalf("relative command = %q", got)
	}
}

func TestToolInfo(t *testing.T) {
	p := &plugin{}
	info := p.toolInfo()
	if info.Capability != "shell" || len(info.Tools) != 1 {
		t.Fatalf("info = %+v", info)
	}
	tool := info.Tools[0]
	if tool.Name != "shell_execute" || tool.Method != "execute" {
		t.Fatalf("tool = %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
		t.Fatalf("parameters: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("schema type = %v", schema["type"])
	}
}
