// Command filetools is a Tools Plugin providing filesystem primitives.
//
// Capability: tools
//   - list: → {"tools":[...]}  (read_file, write_file, edit_file, grep, glob)
//   - call: {"name","arguments"} → {"content", "additionalContexts"}
//
// This plugin is stateless and performs no safety gating.
// Path sandboxing and other constraints are future work (sandbox model TBD).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// defaultReadLines is the limit when the caller omits limit (memory-opt: keep tool results small).
const defaultReadLines = 500

// maxReadLines bounds how many lines read_file returns per call.
const maxReadLines = 5000

// maxGrepResults bounds how many matches grep returns.
const maxGrepResults = 200

// maxLineSize is the maximum byte size for a single scanner line.
const maxLineSize = 1024 * 1024 // 1MB

// newScanner returns a bufio.Scanner configured for large lines.
func newScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	return scanner
}

// requireField returns a FrameError if val is empty.
func requireField(name, val string) error {
	if val == "" {
		return &protocol.FrameError{Code: "bad_arguments", Message: name + " is required"}
	}
	return nil
}

// toolSchemas is the static tool list returned by tools.list.
var toolSchemas = []map[string]any{
	{
		"name":        "read_file",
		"description": "Read a file and return its contents with line numbers (cat -n style). Supports offset and limit for pagination.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]string{"type": "string", "description": "File path to read"},
				"offset": map[string]any{"type": "integer", "description": "Starting line number (0-based, default 0)", "default": 0},
				"limit":  map[string]any{"type": "integer", "description": "Maximum lines to return (default 500, max 5000). Use offset to page.", "default": defaultReadLines},
			},
			"required": []string{"path"},
		},
	},
	{
		"name":        "write_file",
		"description": "Write content to a file. Creates parent directories if needed. Overwrites existing files.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]string{"type": "string", "description": "File path to write"},
				"content": map[string]string{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
	},
	{
		"name":        "edit_file",
		"description": "Edit a file by replacing an exact substring. The old_string must appear exactly once in the file.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]string{"type": "string", "description": "File path to edit"},
				"old_string": map[string]string{"type": "string", "description": "Exact substring to find (must appear exactly once)"},
				"new_string": map[string]string{"type": "string", "description": "Replacement string"},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
	},
	{
		"name":        "grep",
		"description": "Search file contents using a regex pattern. Returns matching lines with file paths and line numbers.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]string{"type": "string", "description": "RE2 regular expression pattern"},
				"path":    map[string]string{"type": "string", "description": "Root directory to search from"},
				"glob":    map[string]string{"type": "string", "description": "Optional filename glob pattern (e.g. *.go)"},
			},
			"required": []string{"pattern"},
		},
	},
	{
		"name":        "glob",
		"description": "Find files matching a glob pattern. Returns matching file paths.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]string{"type": "string", "description": "Glob pattern (e.g. **/*.go, *.txt)"},
				"path":    map[string]string{"type": "string", "description": "Root directory to search from (default: current directory)"},
			},
			"required": []string{"pattern"},
		},
	},
}

// dispatchTool routes a tool call to the matching handler. Exported for test use.
func dispatchTool(name string, args json.RawMessage) (string, error) {
	switch name {
	case "read_file":
		return handleReadFile(args)
	case "write_file":
		return handleWriteFile(args)
	case "edit_file":
		return handleEditFile(args)
	case "grep":
		return handleGrep(args)
	case "glob":
		return handleGlob(args)
	default:
		return "", &protocol.FrameError{Code: "unknown_tool", Message: "unknown tool " + name}
	}
}

func main() {
	s := pluginsdk.New()

	s.Handle("tools", "list", func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(map[string]any{"tools": toolSchemas})
	})

	s.Handle("tools", "call", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		content, err := dispatchTool(in.Name, in.Arguments)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"content": content})
	})

	_ = s.Serve()
}

// ---------------------------------------------------------------------------
// read_file
// ---------------------------------------------------------------------------

func handleReadFile(args json.RawMessage) (string, error) {
	var in struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if err := requireField("path", in.Path); err != nil {
		return "", err
	}
	if in.Limit <= 0 {
		in.Limit = defaultReadLines
	}
	if in.Limit > maxReadLines {
		in.Limit = maxReadLines
	}
	if in.Offset < 0 {
		in.Offset = 0
	}

	f, err := os.Open(in.Path)
	if err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	defer f.Close()

	var lines []string
	scanner := newScanner(f)
	lineNo := 0
	for scanner.Scan() {
		if lineNo >= in.Offset && lineNo < in.Offset+in.Limit {
			lines = append(lines, fmt.Sprintf("%d\t%s", lineNo+1, scanner.Text()))
		}
		lineNo++
		if lineNo >= in.Offset+in.Limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}

	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}

	// If we stopped early, tell the caller.
	if lineNo >= in.Offset+in.Limit {
		// Count remaining lines.
		for scanner.Scan() {
			lineNo++
		}
		remaining := lineNo - in.Offset - len(lines)
		if remaining > 0 {
			fmt.Fprintf(&b, "... (%d more lines; use offset=%d to continue)\n", remaining, in.Offset+len(lines))
		}
	}

	return b.String(), nil
}

// ---------------------------------------------------------------------------
// write_file
// ---------------------------------------------------------------------------

func handleWriteFile(args json.RawMessage) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if err := requireField("path", in.Path); err != nil {
		return "", err
	}

	dir := filepath.Dir(in.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	if err := os.WriteFile(in.Path, []byte(in.Content), 0o644); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}

// ---------------------------------------------------------------------------
// edit_file
// ---------------------------------------------------------------------------

func handleEditFile(args json.RawMessage) (string, error) {
	var in struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if err := requireField("path", in.Path); err != nil {
		return "", err
	}
	if err := requireField("old_string", in.OldString); err != nil {
		return "", err
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}

	content := string(data)
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return "", &protocol.FrameError{Code: "edit_failed", Message: "old_string not found in " + in.Path}
	}
	if count > 1 {
		return "", &protocol.FrameError{Code: "edit_failed", Message: fmt.Sprintf("old_string found %d times in %s; must be unique", count, in.Path)}
	}

	newContent := strings.Replace(content, in.OldString, in.NewString, 1)
	if err := os.WriteFile(in.Path, []byte(newContent), 0o644); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	return fmt.Sprintf("edited %s", in.Path), nil
}

// ---------------------------------------------------------------------------
// grep
// ---------------------------------------------------------------------------

func handleGrep(args json.RawMessage) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Glob    string `json:"glob"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if err := requireField("pattern", in.Pattern); err != nil {
		return "", err
	}
	if in.Path == "" {
		in.Path = "."
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: "invalid regex: " + err.Error()}
	}

	var results []string
	truncated := false

	err = filepath.WalkDir(in.Path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		name := d.Name()
		// Skip hidden files and directories.
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Apply glob filter.
		if in.Glob != "" {
			matched, err := filepath.Match(in.Glob, name)
			if err != nil || !matched {
				return nil
			}
		}
		// Search file contents.
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := newScanner(f)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			if re.MatchString(scanner.Text()) {
				results = append(results, fmt.Sprintf("%s:%d:%s", p, lineNo, scanner.Text()))
				if len(results) >= maxGrepResults {
					truncated = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}

	var b strings.Builder
	for _, r := range results {
		b.WriteString(r)
		b.WriteByte('\n')
	}
	if truncated {
		fmt.Fprintf(&b, "\n... (results truncated at %d matches)\n", maxGrepResults)
	}
	if len(results) == 0 {
		b.WriteString("no matches found\n")
	}
	return b.String(), nil
}

// ---------------------------------------------------------------------------
// glob
// ---------------------------------------------------------------------------

func handleGlob(args json.RawMessage) (string, error) {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	if err := requireField("pattern", in.Pattern); err != nil {
		return "", err
	}
	if in.Path == "" {
		in.Path = "."
	}

	// Support ** by walking the tree ourselves when pattern contains **.
	var matches []string
	if strings.Contains(in.Pattern, "**") {
		// Split pattern into prefix before ** and suffix after **.
		parts := strings.SplitN(in.Pattern, "**", 2)
		suffix := strings.TrimPrefix(parts[1], "/")
		if suffix == "" {
			suffix = "*"
		}

		err := filepath.WalkDir(in.Path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") && d.IsDir() {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			matched, err := filepath.Match(suffix, name)
			if err != nil {
				return nil
			}
			if matched {
				matches = append(matches, p)
			}
			return nil
		})
		if err != nil {
			return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
		}
	} else {
		pattern := filepath.Join(in.Path, in.Pattern)
		var err error
		matches, err = filepath.Glob(pattern)
		if err != nil {
			return "", &protocol.FrameError{Code: "bad_arguments", Message: "invalid glob: " + err.Error()}
		}
	}

	var b strings.Builder
	if len(matches) == 0 {
		b.WriteString("no files found\n")
	} else {
		for _, m := range matches {
			b.WriteString(m)
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}
