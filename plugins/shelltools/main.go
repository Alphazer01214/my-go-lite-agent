// Command shelltools provides a cross-platform shell tool (Phase 1, no PTY).
//
// Capability: tools
//   - list → shell tool schema (not readOnly)
//   - call → run a command under the Session Workspace
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const defaultTimeoutMs = 60000
const maxOutput = 32 * 1024

var toolSchemas = []map[string]any{
	{
		"name":        "shell",
		"description": "Run a shell command in the Session Workspace. Prefer absolute clarity; output is truncated.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":    map[string]string{"type": "string", "description": "Command line to execute"},
				"cwd":        map[string]string{"type": "string", "description": "Working directory relative to Workspace (optional)"},
				"timeoutMs":  map[string]any{"type": "integer", "description": "Timeout in milliseconds (default 60000)"},
			},
			"required": []string{"command"},
		},
	},
}

type pluginConfig struct {
	Shell string `json:"shell"`
}

func configPaths() []string {
	var paths []string
	if exe, err := os.Executable(); err == nil {
		// <exe>/config/shelltools.json (install default layer).
		paths = append(paths, filepath.Join(filepath.Dir(exe), "config", "shelltools.json"))
		paths = append(paths, filepath.Join(filepath.Dir(exe), "config.json"))
	}
	return paths
}

func loadConfig(workspace string) pluginConfig {
	cfg := pluginConfig{}
	for _, p := range configPaths() {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var disk pluginConfig
		if json.Unmarshal(raw, &disk) == nil && disk.Shell != "" {
			cfg.Shell = disk.Shell
		}
	}
	if workspace != "" {
		raw, err := os.ReadFile(filepath.Join(workspace, ".liteagent", "shelltools.json"))
		if err == nil {
			var disk pluginConfig
			if json.Unmarshal(raw, &disk) == nil && disk.Shell != "" {
				cfg.Shell = disk.Shell
			}
		}
	}
	return cfg
}

// defaultShell returns the platform interpreter (ADR/spec: Win powershell, Unix /bin/sh).
func defaultShell(cfg pluginConfig) (string, []string) {
	if cfg.Shell != "" {
		// Allow "bash" / "pwsh" / full path.
		if strings.Contains(cfg.Shell, string(os.PathSeparator)) || strings.Contains(cfg.Shell, "/") {
			return cfg.Shell, []string{"-c"}
		}
		switch strings.ToLower(cfg.Shell) {
		case "powershell", "pwsh", "powershell.exe":
			return cfg.Shell, []string{"-NoProfile", "-Command"}
		case "cmd", "cmd.exe":
			return cfg.Shell, []string{"/C"}
		default:
			return cfg.Shell, []string{"-c"}
		}
	}
	if runtime.GOOS == "windows" {
		return "powershell.exe", []string{"-NoProfile", "-Command"}
	}
	return "/bin/sh", []string{"-c"}
}

func resolveCwd(workspace, cwd string) (string, error) {
	if workspace == "" {
		return "", &protocol.FrameError{Code: "no_workspace", Message: "workspace is required for shell"}
	}
	base, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	if cwd == "" {
		return base, nil
	}
	joined := cwd
	if !filepath.IsAbs(cwd) {
		joined = filepath.Join(base, filepath.FromSlash(cwd))
	}
	joined = filepath.Clean(joined)
	rel, err := filepath.Rel(base, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", &protocol.FrameError{Code: "path_escape", Message: "cwd escapes workspace"}
	}
	return joined, nil
}

func runShell(command, workspace, cwd string, timeoutMs int) (string, error) {
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	dir, err := resolveCwd(workspace, cwd)
	if err != nil {
		return "", err
	}
	cfg := loadConfig(workspace)
	shell, args := defaultShell(cfg)
	args = append(args, command)
	cmd := exec.Command(shell, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", &protocol.FrameError{Code: "shell_start_failed", Message: err.Error()}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		_ = cmd.Process.Kill()
		<-done
		return "", &protocol.FrameError{Code: "timeout", Message: fmt.Sprintf("shell timed out after %dms", timeoutMs)}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exit_code=%d\n", cmd.ProcessState.ExitCode())
	fmt.Fprintf(&b, "shell=%s dir=%s\n", shell, dir)
	out := stdout.String()
	errOut := stderr.String()
	if len(out) > maxOutput {
		out = out[:maxOutput] + "\n... (stdout truncated)\n"
	}
	if len(errOut) > maxOutput {
		errOut = errOut[:maxOutput] + "\n... (stderr truncated)\n"
	}
	if out != "" {
		b.WriteString("stdout:\n")
		b.WriteString(out)
		if !strings.HasSuffix(out, "\n") {
			b.WriteByte('\n')
		}
	}
	if errOut != "" {
		b.WriteString("stderr:\n")
		b.WriteString(errOut)
		if !strings.HasSuffix(errOut, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func main() {
	s := pluginsdk.New()
	s.Handle("tools", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"tools": toolSchemas})
	})
	s.Handle("tools", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Workspace string          `json:"workspace"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if in.Name != "shell" {
			return nil, &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + in.Name}
		}
		var args struct {
			Command   string `json:"command"`
			Cwd       string `json:"cwd"`
			TimeoutMs int    `json:"timeoutMs"`
		}
		if len(in.Arguments) > 0 {
			if err := json.Unmarshal(in.Arguments, &args); err != nil {
				return nil, &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
			}
		}
		if strings.TrimSpace(args.Command) == "" {
			return nil, &protocol.FrameError{Code: "bad_arguments", Message: "command is required"}
		}
		content, err := runShell(args.Command, in.Workspace, args.Cwd, args.TimeoutMs)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"content": content})
	})
	_ = s.Serve()
}
