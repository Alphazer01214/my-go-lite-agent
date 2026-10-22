package main

import (
	"encoding/json"
)

func definitions() []Definition {
	return []Definition{
		{
			Name: "file_read", Risk: RiskLow, ParallelSafe: true, TargetCapability: "file", TargetMethod: "read",
			Tags:        []string{"fs", "read"},
			Description: "Read UTF-8 text from a file inside the workspace. Supports a line range and byte limit.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string","description":"Workspace-relative path"},
					"start_line":{"type":"integer","minimum":1},
					"end_line":{"type":"integer","minimum":1},
					"max_bytes":{"type":"integer","minimum":1,"maximum":1048576}
				},
				"required":["path"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_write", Risk: RiskHigh, TargetCapability: "file", TargetMethod: "write",
			Tags:        []string{"fs", "write"},
			Description: "Atomically write UTF-8 text to a workspace file. Creates missing parent directories unless create_dirs is false. Use expected_sha256 for conflict detection.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string","description":"Workspace-relative file path"},
					"content":{"type":"string"},
					"create_dirs":{"type":"boolean","default":true,"description":"Create missing parent directories. Set false to fail with path_not_found instead."},
					"overwrite":{"type":"boolean","default":true,"description":"Set false to fail with file_exists when the file is already present."},
					"expected_sha256":{"type":"string","pattern":"^[0-9a-fA-F]{64}$","description":"Fail with file_conflict unless the current file digest matches."}
				},
				"required":["path","content"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_list", Risk: RiskLow, ParallelSafe: true, TargetCapability: "file", TargetMethod: "list",
			Tags:        []string{"fs", "read"},
			Description: "List files and directories inside the workspace without following symlinked directories.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string","default":"."},
					"recursive":{"type":"boolean","default":false},
					"max_depth":{"type":"integer","minimum":1,"maximum":16},
					"max_entries":{"type":"integer","minimum":1,"maximum":1000},
					"include_hidden":{"type":"boolean","default":true}
				},
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_stat", Risk: RiskLow, ParallelSafe: true, TargetCapability: "file", TargetMethod: "stat",
			Tags:        []string{"fs", "read"},
			Description: "Inspect one workspace path, including type, size, mode, mtime and optional SHA-256.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"path":{"type":"string"},"hash":{"type":"boolean","default":false}},
				"required":["path"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_search", Risk: RiskLow, ParallelSafe: true, TargetCapability: "file", TargetMethod: "search",
			Tags:        []string{"fs", "read"},
			Description: "Search UTF-8 workspace text using a literal string or Go regular expression.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"pattern":{"type":"string","minLength":1,"maxLength":1024},
					"path":{"type":"string","default":"."},
					"glob":{"type":"string"},
					"regex":{"type":"boolean","default":false},
					"case_sensitive":{"type":"boolean","default":true},
					"max_matches":{"type":"integer","minimum":1,"maximum":1000},
					"max_files":{"type":"integer","minimum":1,"maximum":10000},
					"max_file_bytes":{"type":"integer","minimum":1,"maximum":1048576},
					"context_lines":{"type":"integer","minimum":0,"maximum":5}
				},
				"required":["pattern"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_glob", Risk: RiskLow, ParallelSafe: true, TargetCapability: "file", TargetMethod: "glob",
			Tags:        []string{"fs", "read"},
			Description: "Find workspace files by glob. Supports *, ? and ** and returns sorted relative paths.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"pattern":{"type":"string","minLength":1},
					"max_results":{"type":"integer","minimum":1,"maximum":1000}
				},
				"required":["pattern"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_replace", Risk: RiskHigh, TargetCapability: "file", TargetMethod: "replace",
			Tags:        []string{"fs", "write"},
			Description: "Replace literal text in one workspace file atomically. Ambiguous matches fail unless replace_all is true.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string"},
					"old_text":{"type":"string","minLength":1},
					"new_text":{"type":"string"},
					"replace_all":{"type":"boolean","default":false},
					"expected_sha256":{"type":"string","pattern":"^[0-9a-fA-F]{64}$"}
				},
				"required":["path","old_text","new_text"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_patch", Risk: RiskHigh, TargetCapability: "file", TargetMethod: "patch",
			Tags:        []string{"fs", "write"},
			Description: "Apply ordered literal edits to one workspace file. The whole patch is atomic.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string"},
					"edits":{"type":"array","minItems":1,"maxItems":32,"items":{
						"type":"object",
						"properties":{
							"old_text":{"type":"string","minLength":1},
							"new_text":{"type":"string"},
							"replace_all":{"type":"boolean","default":false}
						},
						"required":["old_text","new_text"],
						"additionalProperties":false
					}},
					"expected_sha256":{"type":"string","pattern":"^[0-9a-fA-F]{64}$"}
				},
				"required":["path","edits"],
				"additionalProperties":false
			}`),
		},
	}
}
