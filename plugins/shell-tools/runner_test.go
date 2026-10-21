package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("LITEAGENT_SHELL_HELPER") != "1" {
		return
	}
	switch os.Getenv("LITEAGENT_SHELL_MODE") {
	case "print":
		fmt.Print("hello")
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, "failed")
		os.Exit(7)
	case "sleep":
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "large":
		fmt.Print(strings.Repeat("x", 1000))
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func helperSpec(mode string, timeout int, limit int) processSpec {
	return processSpec{
		Command:   os.Args[0],
		Args:      []string{"-test.run=TestHelperProcess", "--"},
		Env:       append(os.Environ(), "LITEAGENT_SHELL_HELPER=1", "LITEAGENT_SHELL_MODE="+mode),
		TimeoutMS: timeout, MaxOutputBytes: limit,
	}
}

func TestRunProcessSuccessFailureAndTruncation(t *testing.T) {
	raw, err := runProcess(context.Background(), helperSpec("print", 2000, 100))
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseResult(raw)
	if err != nil || result.IsError || !strings.Contains(result.Content, "hello") {
		t.Fatalf("success = %+v, %v", result, err)
	}

	raw, err = runProcess(context.Background(), helperSpec("fail", 2000, 100))
	if err != nil {
		t.Fatal(err)
	}
	result, _ = ParseResult(raw)
	if !result.IsError || result.Meta["exit_code"] != float64(7) {
		t.Fatalf("failure = %+v", result)
	}

	raw, err = runProcess(context.Background(), helperSpec("large", 2000, 10))
	if err != nil {
		t.Fatal(err)
	}
	result, _ = ParseResult(raw)
	if !result.Truncated || result.Meta["stdout_truncated"] != true {
		t.Fatalf("truncate = %+v", result)
	}
}

func TestRunProcessTimeoutAndCancellation(t *testing.T) {
	raw, err := runProcess(context.Background(), helperSpec("sleep", 50, 100))
	if err != nil {
		t.Fatal(err)
	}
	result, _ := ParseResult(raw)
	if !result.IsError || result.Meta["timed_out"] != true {
		t.Fatalf("timeout = %+v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err = runProcess(ctx, helperSpec("sleep", 5000, 100))
	if pluginsdk.Code(err) != pluginsdk.CodeCallCancelled {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestInterpreterAndEnvironmentValidation(t *testing.T) {
	if !blockedInterpreter("powershell.exe") || !blockedInterpreter("/bin/bash") {
		t.Fatal("interpreter not blocked")
	}
	if _, err := mergedEnv(map[string]string{"BAD-NAME": "x"}); err == nil {
		t.Fatal("bad env name accepted")
	}
}

func TestExecuteRejectsRelativeWorkspace(t *testing.T) {
	p := &plugin{}
	_, err := p.execute(context.Background(), ProviderInput{Workspace: "relative"},
		executeIn{Command: "go"})
	if pluginsdk.Code(err) != "workspace_invalid" {
		t.Fatalf("error = %v", err)
	}
}

func TestExecutePassesArgumentsWithoutShell(t *testing.T) {
	dir := t.TempDir()
	raw, err := (&plugin{}).execute(context.Background(), ProviderInput{Workspace: dir}, executeIn{
		Command: "go", Args: []string{"env", "GOVERSION"}, TimeoutMS: intPtr(5000),
	})
	if err != nil {
		t.Fatal(err)
	}
	var result Result
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "go") {
		t.Fatalf("result = %+v", result)
	}
}

func intPtr(value int) *int { return &value }

func TestResolveWorkspaceDirAcceptsAbsoluteInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	resolved, err := resolveWorkspaceDir(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(resolved, want) {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}

	outside := t.TempDir()
	if _, err := resolveWorkspaceDir(dir, outside); pluginsdk.Code(err) != "cwd_escape" {
		t.Fatalf("outside cwd error = %v", err)
	}
}

func TestDefinitionsDescribePlatformAndCwd(t *testing.T) {
	defs := definitions(config{AllowShellInterpreters: true})
	if len(defs) != 1 {
		t.Fatalf("definitions = %+v", defs)
	}
	description := defs[0].Description
	if !strings.Contains(description, "Platform: "+runtime.GOOS) ||
		!strings.Contains(description, "No shell parsing") ||
		!strings.Contains(description, "workspace file operations") {
		t.Fatalf("description = %q", description)
	}
	if !strings.Contains(string(defs[0].InputSchema), "Absolute paths inside the workspace") {
		t.Fatalf("schema = %s", defs[0].InputSchema)
	}
}
