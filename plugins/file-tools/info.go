package main

import "encoding/json"

var (
	readParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Workspace-relative file path."},
    "start_line": {"type": "integer", "minimum": 1},
    "end_line": {"type": "integer", "minimum": 1},
    "max_bytes": {"type": "integer", "minimum": 1, "maximum": 1048576}
  },
  "required": ["path"],
  "additionalProperties": false
}`)

	writeParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Workspace-relative file path."},
    "content": {"type": "string"},
    "create_dirs": {"type": "boolean"},
    "overwrite": {"type": "boolean"},
    "expected_sha256": {"type": "string"}
  },
  "required": ["path", "content"],
  "additionalProperties": false
}`)

	globParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "Workspace-relative glob; * and ? do not cross directories, ** does."},
    "max_results": {"type": "integer", "minimum": 1, "maximum": 1000}
  },
  "required": ["pattern"],
  "additionalProperties": false
}`)

	grepParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {"type": "string"},
    "path": {"type": "string", "description": "Workspace-relative file or directory; defaults to ."},
    "glob": {"type": "string", "description": "Optional workspace-relative glob filter."},
    "regex": {"type": "boolean"},
    "case_sensitive": {"type": "boolean"},
    "max_matches": {"type": "integer", "minimum": 1, "maximum": 1000},
    "max_files": {"type": "integer", "minimum": 1},
    "max_file_bytes": {"type": "integer", "minimum": 1, "maximum": 1048576}
  },
  "required": ["pattern"],
  "additionalProperties": false
}`)

	editParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {"type": "string", "description": "Workspace-relative file path."},
    "old_text": {"type": "string"},
    "new_text": {"type": "string"},
    "replace_all": {"type": "boolean"},
    "expected_sha256": {"type": "string"}
  },
  "required": ["path", "old_text", "new_text"],
  "additionalProperties": false
}`)
)

func (p *plugin) toolInfo() infoResponse {
	return infoResponse{
		Capability: "file",
		Tools: []toolInfo{
			{
				Name:        "file_read",
				Description: "Read a UTF-8 text file inside the workspace, optionally a line range.",
				Parameters:  readParameters,
				Method:      "read",
			},
			{
				Name:        "file_write",
				Description: "Write a UTF-8 text file inside the workspace, creating parent directories when requested.",
				Parameters:  writeParameters,
				Method:      "write",
			},
			{
				Name:        "file_glob",
				Description: "List workspace files matching a glob pattern.",
				Parameters:  globParameters,
				Method:      "glob",
			},
			{
				Name:        "file_grep",
				Description: "Search UTF-8 text files inside the workspace for a literal string or regular expression.",
				Parameters:  grepParameters,
				Method:      "grep",
			},
			{
				Name:        "file_edit",
				Description: "Replace text in a UTF-8 file inside the workspace.",
				Parameters:  editParameters,
				Method:      "edit",
			},
		},
	}
}
