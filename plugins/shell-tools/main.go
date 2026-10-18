package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
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
		input, arguments, err := toolcontract.Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		var in executeIn
		if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
			return nil, err
		}
		return p.execute(ctx, input, in)
	}).WithInfo("execute", "execute one process without a shell"))
	go func() {
		if err := toolcontract.RegisterAll(context.Background(), sdk, definitions()); err != nil {
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

func definitions() []toolcontract.Definition {
	return []toolcontract.Definition{{
		Name: "shell_execute", Risk: toolcontract.RiskHigh,
		TargetCapability: "shell", TargetMethod: "execute",
		Description: "Execute one process with argument-vector semantics. No shell parsing is performed. " +
			"Returns exit code, stdout, stderr, duration and truncation metadata.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"command":{"type":"string","minLength":1},
				"args":{"type":"array","items":{"type":"string"}},
				"cwd":{"type":"string","default":"."},
				"env":{"type":"object","additionalProperties":{"type":"string"}},
				"timeout_ms":{"type":"integer","minimum":1,"maximum":600000},
				"max_output_bytes":{"type":"integer","minimum":1,"maximum":4194304}
			},
			"required":["command"],
			"additionalProperties":false
		}`),
	}}
}

func (p *plugin) execute(ctx context.Context, input toolcontract.ProviderInput, in executeIn) (json.RawMessage, error) {
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
		return toolcontract.Failure("command_blocked", "shell interpreters are disabled by configuration", nil)
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

func resolveWorkspaceDir(workspace, relative string) (string, error) {
	normalized := strings.ReplaceAll(relative, `\`, "/")
	if strings.HasPrefix(normalized, "/") || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" {
		return "", pluginsdk.ErrCode("cwd_invalid", "cwd must be workspace-relative")
	}
	if normalized != "." {
		for _, part := range strings.Split(normalized, "/") {
			if part == "" || part == "." || part == ".." {
				return "", pluginsdk.ErrCode("cwd_invalid", "cwd contains an invalid component")
			}
		}
	}
	candidate := filepath.Join(workspace, filepath.FromSlash(normalized))
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
