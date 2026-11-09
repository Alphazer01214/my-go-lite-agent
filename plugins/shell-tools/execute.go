package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func (p *plugin) execute(ctx context.Context, req *providerRequest, in *executeArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}
	command := strings.TrimSpace(in.Command)
	if command == "" {
		return toolResult{}, badArguments("command is required")
	}
	if strings.ContainsRune(command, 0) {
		return toolResult{}, badArguments("command contains NUL")
	}
	for index, arg := range in.Args {
		if strings.ContainsRune(arg, 0) {
			return toolResult{}, badArguments("args[%d] contains NUL", index)
		}
	}
	if len(in.Stdin) > maxStdinBytes {
		return toolResult{}, badArguments("stdin exceeds %d bytes", maxStdinBytes)
	}

	cwd, err := resolveCwd(req.Workspace, in.Cwd)
	if err != nil {
		return toolResult{}, err
	}
	command = resolveCommand(command, cwd)
	if strings.ContainsAny(command, " \t") {
		if _, err := os.Stat(command); err != nil {
			return toolResult{}, badArguments(
				"command must be one executable; pass arguments in args[] (got %q)", in.Command)
		}
	}

	env, err := mergedEnv(in.Env)
	if err != nil {
		return toolResult{}, err
	}

	timeoutMS := in.TimeoutMS
	if timeoutMS == 0 {
		timeoutMS = defaultTimeoutMS
	}
	if timeoutMS < 0 {
		return toolResult{}, badArguments("timeout_ms must be positive")
	}
	if timeoutMS > maxTimeoutMS {
		timeoutMS = maxTimeoutMS
	}

	maxOutput := in.MaxOutputBytes
	if maxOutput == 0 {
		maxOutput = defaultMaxOutputBytes
	}
	if maxOutput < 0 {
		return toolResult{}, badArguments("max_output_bytes must be positive")
	}
	if maxOutput > maxMaxOutputBytes {
		maxOutput = maxMaxOutputBytes
	}

	return runProcess(ctx, processSpec{
		Command:        command,
		Args:           append([]string(nil), in.Args...),
		Dir:            cwd,
		Env:            env,
		Stdin:          in.Stdin,
		TimeoutMS:      timeoutMS,
		MaxOutputBytes: maxOutput,
	})
}

func resolveCwd(workspace, raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", errCwdInvalid("cwd contains NUL")
	}
	if raw == "" {
		raw = "."
	}

	var path string
	if filepath.IsAbs(raw) {
		path = filepath.Clean(raw)
	} else {
		base := workspace
		if base == "" {
			base = "."
		}
		path = filepath.Join(base, raw)
		if !filepath.IsAbs(path) {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return "", errCwdInvalid(err.Error())
			}
			path = absolute
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", errCwdInvalid(fmt.Sprintf("cwd is not accessible: %v", err))
	}
	if !info.IsDir() {
		return "", errCwdInvalid("cwd is not a directory: " + raw)
	}
	return path, nil
}

func resolveCommand(command, cwd string) string {
	if filepath.IsAbs(command) {
		return command
	}
	if hasPathSeparator(command) {
		return filepath.Join(cwd, command)
	}
	return command
}

func hasPathSeparator(name string) bool {
	if strings.ContainsRune(name, filepath.Separator) {
		return true
	}
	return runtime.GOOS == "windows" && strings.ContainsRune(name, '/')
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
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
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
