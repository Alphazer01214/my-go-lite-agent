package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type config struct {
	AllowShellInterpreters bool `json:"allow_shell_interpreters"`
}

type plugin struct {
	cfg config
}

func loadConfig() config {
	var cfg config
	data, err := os.ReadFile("config.json")
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg
}

func main() {
	sdk := pluginsdk.NewPlugin("shell-tools")
	p := &plugin{cfg: loadConfig()}
	sdk.Register("shell", "execute", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		input, arguments, err := Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		var in executeIn
		if err := DecodeStrict(arguments, &in); err != nil {
			return nil, err
		}
		return p.execute(ctx, input, in)
	}).WithInfo("execute", "execute one process without a shell"))
	go func() {
		if err := SyncAll(context.Background(), sdk, definitions(p.cfg)); err != nil {
			fmt.Fprintln(os.Stderr, "shell-tools register:", err)
		}
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type executeIn struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutMS      *int              `json:"timeout_ms,omitempty"`
	MaxOutputBytes *int              `json:"max_output_bytes,omitempty"`
}

func definitions(cfg config) []Definition {
	interpreterHint := " Shell interpreters (" + interpreterNames() + ") are disabled by configuration."
	if cfg.AllowShellInterpreters {
		switch runtime.GOOS {
		case "windows":
			interpreterHint = ` Shell syntax needs an explicit interpreter, e.g. command "cmd.exe" with args ["/c","dir"] or command "powershell.exe" with args ["-NoProfile","-Command","Get-ChildItem"].`
		default:
			interpreterHint = ` Shell syntax needs an explicit interpreter, e.g. command "/bin/sh" with args ["-c","ls -la"].`
		}
	}
	return []Definition{{
		Name: "shell_execute", Risk: RiskCritical, Tags: []string{"shell"},
		TargetCapability: "shell", TargetMethod: "execute",
		Description: "Execute one executable process and return exit code, stdout, stderr, duration and truncation metadata. " +
			"No shell parsing is performed: pipes, redirection, && and shell built-ins are not interpreted." +
			interpreterHint +
			" Platform: " + runtime.GOOS + ". " +
			"Use file_list, file_read and file_search for workspace file operations; use this tool to build, test and run programs.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"command":{"type":"string","minLength":1,"description":"Executable name or path. Shell built-ins and operators are not interpreted."},
				"args":{"type":"array","items":{"type":"string"},"description":"Arguments passed to the executable verbatim."},
				"cwd":{"type":"string","default":".","description":"Workspace-relative directory. Absolute paths inside the workspace are also accepted. Defaults to the workspace root."},
				"env":{"type":"object","additionalProperties":{"type":"string"},"description":"Environment variable overrides for this process."},
				"timeout_ms":{"type":"integer","minimum":1,"maximum":600000,"description":"Process timeout in milliseconds. Default 120000."},
				"max_output_bytes":{"type":"integer","minimum":1,"maximum":4194304,"description":"Per-stream output cap in bytes. Default 1048576."}
			},
			"required":["command"],
			"additionalProperties":false
		}`),
	}}
}

func interpreterNames() string {
	if runtime.GOOS == "windows" {
		return "cmd, powershell, pwsh, bash, sh, zsh, fish, wsl"
	}
	return "bash, sh, zsh, fish"
}

func (p *plugin) execute(ctx context.Context, input ProviderInput, in executeIn) (json.RawMessage, error) {
	if input.Workspace == "" {
		return nil, pluginsdk.ErrCode("workspace_required", "workspace is required")
	}
	if !filepath.IsAbs(input.Workspace) {
		return nil, pluginsdk.ErrCode("workspace_invalid", "workspace must be absolute")
	}
	workspaceInfo, err := os.Stat(input.Workspace)
	if err != nil || !workspaceInfo.IsDir() {
		return nil, pluginsdk.ErrCode("workspace_invalid", "workspace must be an existing directory")
	}
	if strings.TrimSpace(in.Command) == "" || strings.ContainsRune(in.Command, 0) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "command is required")
	}
	if !p.cfg.AllowShellInterpreters && blockedInterpreter(in.Command) {
		return Failure("command_blocked", "shell interpreters are disabled by configuration", nil)
	}
	if in.Cwd == "" {
		in.Cwd = "."
	}
	cwd, err := resolveWorkspaceDir(input.Workspace, in.Cwd)
	if err != nil {
		return nil, err
	}
	env, err := mergedEnv(in.Env)
	if err != nil {
		return nil, err
	}
	timeoutMS := 120000
	if in.TimeoutMS != nil {
		timeoutMS = *in.TimeoutMS
	}
	maxOutput := 1 << 20
	if in.MaxOutputBytes != nil {
		maxOutput = *in.MaxOutputBytes
	}
	return runProcess(ctx, processSpec{
		Command: in.Command, Args: in.Args, Dir: cwd, Env: env,
		TimeoutMS: timeoutMS, MaxOutputBytes: maxOutput,
	})
}

func resolveWorkspaceDir(workspace, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "."
	}
	normalized := strings.ReplaceAll(requested, `\`, "/")
	var candidate string
	if filepath.IsAbs(requested) {
		candidate = filepath.Clean(requested)
	} else {
		if filepath.VolumeName(requested) != "" {
			// Drive-relative forms such as C:foo have no single meaning here.
			return "", pluginsdk.ErrCode("cwd_invalid", "cwd must be workspace-relative or an absolute path")
		}
		if normalized != "." {
			for _, part := range strings.Split(normalized, "/") {
				if part == "" || part == "." || part == ".." {
					return "", pluginsdk.ErrCode("cwd_invalid", "cwd contains an invalid component")
				}
			}
		}
		candidate = filepath.Join(workspace, filepath.FromSlash(normalized))
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", pluginsdk.ErrCode("cwd_invalid", err.Error())
	}
	workspaceResolved, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", pluginsdk.ErrCode("workspace_invalid", err.Error())
	}
	rel, err := filepath.Rel(workspaceResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", pluginsdk.ErrCode("cwd_escape", "cwd escapes the workspace")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", pluginsdk.ErrCode("cwd_invalid", "cwd is not a directory")
	}
	return resolved, nil
}

func mergedEnv(overrides map[string]string) ([]string, error) {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		if !validEnvName(key) || strings.ContainsRune(value, 0) {
			return nil, pluginsdk.ErrCode("env_invalid", "invalid environment variable: "+key)
		}
		values[key] = value
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out, nil
}

func validEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' {
			continue
		}
		if index > 0 && char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func blockedInterpreter(command string) bool {
	name := strings.ToLower(filepath.Base(command))
	name = strings.TrimSuffix(name, ".exe")
	switch name {
	case "cmd", "powershell", "pwsh", "bash", "sh", "zsh", "fish", "wsl", "cscript", "wscript":
		return true
	default:
		return false
	}
}
