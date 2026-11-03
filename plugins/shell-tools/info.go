package main

import (
	"encoding/json"
	"runtime"
)

var executeParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "minLength": 1, "description": "Executable name or path. No shell parsing: pipes, redirection, && and shell built-ins are not interpreted."},
    "args": {"type": "array", "items": {"type": "string"}, "description": "Arguments passed to the executable verbatim."},
    "cwd": {"type": "string", "default": ".", "description": "Working directory. Relative paths are resolved against the workspace."},
    "env": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Environment variable overrides for this process."},
    "stdin": {"type": "string", "description": "Text written to the process stdin. Maximum 1048576 bytes."},
    "timeout_ms": {"type": "integer", "minimum": 1, "maximum": 600000, "description": "Process timeout in milliseconds. Default 120000."},
    "max_output_bytes": {"type": "integer", "minimum": 1, "maximum": 4194304, "description": "Per-stream output cap in bytes. Default 1048576."}
  },
  "required": ["command"],
  "additionalProperties": false
}`)

func (p *plugin) toolInfo() infoResponse {
	hint := "call an interpreter such as /bin/sh -c explicitly when shell syntax is needed."
	if runtime.GOOS == "windows" {
		hint = "call cmd.exe /c or powershell.exe -Command explicitly when shell syntax is needed."
	}
	description := "Execute one executable process and return exit code, stdout, stderr, duration and truncation metadata. " +
		"No shell parsing is performed: pipes, redirection, && and shell built-ins are not interpreted; " +
		hint + " " +
		"Platform: " + runtime.GOOS + ". " +
		"Use the file tools for workspace file operations."
	return infoResponse{
		Capability: "shell",
		Tools: []toolInfo{{
			Name:        "shell_execute",
			Description: description,
			Parameters:  executeParameters,
			Method:      "execute",
		}},
	}
}
