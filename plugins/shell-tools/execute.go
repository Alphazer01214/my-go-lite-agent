package main

import (
	"context"
	"encoding/json"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"os"
	"path/filepath"
	"strings"
)

type executeIn struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	TimeoutMS      *int              `json:"timeout_ms,omitempty"`
	MaxOutputBytes *int              `json:"max_output_bytes,omitempty"`
}

func (p *plugin) execute(ctx context.Context, input ProviderInput, in executeIn) (json.RawMessage, error) {
	if input.Workspace == "" {
		return nil, errWorkspaceRequired("workspace is required")
	}
	if !filepath.IsAbs(input.Workspace) {
		return nil, errWorkspaceInvalid("workspace must be absolute")
	}
	workspaceInfo, err := os.Stat(input.Workspace)
	if err != nil || !workspaceInfo.IsDir() {
		return nil, errWorkspaceInvalid("workspace must be an existing directory")
	}
	if strings.TrimSpace(in.Command) == "" || strings.ContainsRune(in.Command, 0) {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "command is required")
	}
	if !p.cfg.AllowShellInterpreters && blockedInterpreter(in.Command) {
		return Failure(codeCommandBlocked, "shell interpreters are disabled by configuration", nil)
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
			return "", errCwdInvalid("cwd must be workspace-relative or an absolute path")
		}
		if normalized != "." {
			for _, part := range strings.Split(normalized, "/") {
				if part == "" || part == "." || part == ".." {
					return "", errCwdInvalid("cwd contains an invalid component")
				}
			}
		}
		candidate = filepath.Join(workspace, filepath.FromSlash(normalized))
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", errCwdInvalid(err.Error())
	}
	workspaceResolved, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", errWorkspaceInvalid(err.Error())
	}
	rel, err := filepath.Rel(workspaceResolved, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errCwdEscape("cwd escapes the workspace")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errCwdInvalid("cwd is not a directory")
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
			return nil, errEnvInvalid("invalid environment variable: " + key)
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
