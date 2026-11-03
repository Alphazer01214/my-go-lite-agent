package main

import (
	"encoding/json"
	"fmt"
	"runtime"
)

var executeParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "code": {"type": "string", "minLength": 1, "maxLength": 524288, "description": "Python source to execute. Mutually exclusive with path."},
    "path": {"type": "string", "minLength": 1, "description": "Path to a Python file. Relative paths resolve against the workspace. Mutually exclusive with code."},
    "args": {"type": "array", "items": {"type": "string"}, "description": "Arguments passed to the program. In code mode they become sys.argv[1:]."},
    "cwd": {"type": "string", "default": ".", "description": "Working directory. Relative paths resolve against the workspace."},
    "env": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Environment variable overrides for this process."},
    "timeout_ms": {"type": "integer", "minimum": 1, "maximum": 600000, "description": "Process timeout in milliseconds. Default 120000."},
    "max_output_bytes": {"type": "integer", "minimum": 1, "maximum": 4194304, "description": "Per-stream output cap in bytes. Default 1048576."}
  },
  "additionalProperties": false
}`)

func (p *plugin) toolInfo() infoResponse {
	description := fmt.Sprintf(
		"Run Python code or a script. code mode executes the source through the configured interpreter's stdin; "+
			"path mode executes a file. Relative paths resolve against the workspace. "+
			"Returns exit code, stdout, stderr, duration and truncation metadata. "+
			"Interpreter: %s. Platform: %s.",
		p.interpreter, runtime.GOOS,
	)
	if !p.interpreterFound {
		description += " The interpreter was not found at startup; set interpreter in config.json if calls fail."
	}
	return infoResponse{
		Capability: "python",
		Tools: []toolInfo{{
			Name:        "python_execute",
			Description: description,
			Parameters:  executeParameters,
			Method:      "execute",
		}},
	}
}
