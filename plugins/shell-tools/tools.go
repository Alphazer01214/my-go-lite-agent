package main

import (
	"encoding/json"
	"runtime"
)

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
