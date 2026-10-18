package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type toolHandler func(context.Context, *pluginsdk.Request, toolcontract.ProviderInput, json.RawMessage) (json.RawMessage, error)

func handler(fn toolHandler) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		input, arguments, err := toolcontract.Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		return fn(ctx, req, input, arguments)
	}
}

func main() {
	sdk := pluginsdk.NewPlugin("file-tools")
	p := newPlugin()

	sdk.Register("file", "read", pluginsdk.NewHandler(handler(p.read)).WithInfo("read", "read a workspace file"))
	sdk.Register("file", "write", pluginsdk.NewHandler(handler(p.write)).WithInfo("write", "atomically write a workspace file"))
	sdk.Register("file", "list", pluginsdk.NewHandler(handler(p.list)).WithInfo("list", "list workspace entries"))
	sdk.Register("file", "stat", pluginsdk.NewHandler(handler(p.stat)).WithInfo("stat", "inspect a workspace path"))
	sdk.Register("file", "search", pluginsdk.NewHandler(handler(p.search)).WithInfo("search", "search workspace text"))
	sdk.Register("file", "glob", pluginsdk.NewHandler(handler(p.glob)).WithInfo("glob", "match workspace paths"))
	sdk.Register("file", "replace", pluginsdk.NewHandler(handler(p.replace)).WithInfo("replace", "replace literal text atomically"))
	sdk.Register("file", "patch", pluginsdk.NewHandler(handler(p.patch)).WithInfo("patch", "apply ordered literal edits atomically"))

	go func() {
		if err := toolcontract.RegisterAll(context.Background(), sdk, definitions()); err != nil {
			fmt.Fprintln(os.Stderr, "file-tools register:", err)
		}
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type plugin struct{}

func newPlugin() *plugin { return &plugin{} }

func definitions() []toolcontract.Definition {
	return []toolcontract.Definition{
		{
			Name: "file_read", Risk: toolcontract.RiskLow, TargetCapability: "file", TargetMethod: "read",
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
			Name: "file_write", Risk: toolcontract.RiskHigh, TargetCapability: "file", TargetMethod: "write",
			Description: "Atomically write UTF-8 text to a workspace file. Use expected_sha256 for conflict detection.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"path":{"type":"string"},
					"content":{"type":"string"},
					"create_dirs":{"type":"boolean","default":false},
					"overwrite":{"type":"boolean","default":true},
					"expected_sha256":{"type":"string","pattern":"^[0-9a-fA-F]{64}$"}
				},
				"required":["path","content"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_list", Risk: toolcontract.RiskLow, TargetCapability: "file", TargetMethod: "list",
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
			Name: "file_stat", Risk: toolcontract.RiskLow, TargetCapability: "file", TargetMethod: "stat",
			Description: "Inspect one workspace path, including type, size, mode, mtime and optional SHA-256.",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"path":{"type":"string"},"hash":{"type":"boolean","default":false}},
				"required":["path"],
				"additionalProperties":false
			}`),
		},
		{
			Name: "file_search", Risk: toolcontract.RiskLow, TargetCapability: "file", TargetMethod: "search",
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
			Name: "file_glob", Risk: toolcontract.RiskLow, TargetCapability: "file", TargetMethod: "glob",
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
			Name: "file_replace", Risk: toolcontract.RiskHigh, TargetCapability: "file", TargetMethod: "replace",
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
			Name: "file_patch", Risk: toolcontract.RiskHigh, TargetCapability: "file", TargetMethod: "patch",
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
