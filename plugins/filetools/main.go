// Command filetools is a Tools Plugin providing filesystem primitives.
//
// Capability: tools
//   - list: → {"tools":[...]}  (read_file, write_file, edit_file, grep, glob)
//   - call: {"name","arguments","workspace"} → {"content", "additionalContexts"}
//
// Paths resolve against the Session Workspace (ADR-0020). Hard path-escape
// rejection is defense-in-depth; product policy lives in the policy plugin
// (ADR-0019).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

func newScanner(f *os.File) *bufio.Scanner {
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	return scanner
}

func requireField(name, val string) error {
	if val == "" {
		return &protocol.FrameError{Code: "bad_arguments", Message: name + " is required"}
	}
	return nil
}

// resolvePath joins a relative path onto Workspace and rejects escapes (ADR-0020).
func resolvePath(workspace, p string) (string, error) {
	if err := requireField("path", p); err != nil {
		return "", err
	}
	if workspace == "" {
		// No Session Workspace: only absolute paths are safe (do not guess cwd).
		if !filepath.IsAbs(p) {
			return "", &protocol.FrameError{Code: "no_workspace", Message: "relative path requires workspace"}
		}
		return filepath.Clean(p), nil
	}
	base, err := filepath.Abs(workspace)
	if err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	var abs string
	if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Clean(filepath.Join(base, filepath.FromSlash(p)))
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", &protocol.FrameError{Code: "path_escape", Message: "path escapes workspace: " + p}
	}
	return abs, nil
}

// toolSchemas is the static tool list returned by tools.list.
// Read-only tools carry readOnly:true for parallel scheduling (ADR/spec).
var toolSchemas = []map[string]any{
	{
		"name":        "read_file",
		"description": "Read a file and return its contents with line numbers (cat -n style). Supports offset and limit for pagination.",
		"readOnly":    true,
		"severity":    "low",
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
		"severity":    "medium",
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
		"severity":    "medium",
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
		"readOnly":    true,
		"severity":    "low",
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
		"readOnly":    true,
		"severity":    "low",
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

func dispatchTool(name string, args json.RawMessage, workspace string) (string, error) {
	switch name {
	case "read_file":
		return handleReadFile(args, workspace)
	case "write_file":
		return handleWriteFile(args, workspace)
	case "edit_file":
		return handleEditFile(args, workspace)
	case "grep":
		return handleGrep(args, workspace)
	case "glob":
		return handleGlob(args, workspace)
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
			Workspace string          `json:"workspace"`
		}
		if err := json.Unmarshal(req.Payload, &in); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		content, err := dispatchTool(in.Name, in.Arguments, in.Workspace)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"content": content})
	})

	// workspace.resolve maps a directory NAME picked in the Web Shell to an
	// absolute path (ADR-0020). The File System Access API never reveals the
	// real path; this Plugin owns the directory walk so Host does not traverse
	// the filesystem for a plugin's business (ADR-0026).
	s.Handle("workspace", "resolve", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in struct {
			Name string `json:"name"`
		}
		if len(req.Payload) > 0 {
			_ = json.Unmarshal(req.Payload, &in)
		}
		return resolveWorkspaceName(in.Name)
	})

	_ = s.Serve()
}

// workspaceResolveMaxDirs caps the directory walk so a pick never hangs.
const workspaceResolveMaxDirs = 20000

// skipWalkDirs are directory names never searched for a picked workspace.
var skipWalkDirs = map[string]bool{
	"node_modules": true, "AppData": true, "$RECYCLE.BIN": true,
	"System Volume Information": true, "Windows": true, "ProgramData": true,
}

// resolveWorkspaceName implements workspace.resolve: an absolute path is taken
// at face value; otherwise the name is matched against the Plugin's working
// directory (descendants, depth<=3) and its ancestors' immediate children, so
// picking a sibling project works. Exactly one match wins; several become
// candidates; zero means the user types the path.
func resolveWorkspaceName(name string) (json.RawMessage, error) {
	raw := strings.TrimSpace(name)
	out := map[string]any{"name": raw, "path": "", "candidates": []string{}}
	if raw == "" {
		return json.Marshal(out)
	}
	// An absolute (or otherwise usable) path is taken at face value.
	if abs, err := filepath.Abs(raw); err == nil {
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			out["path"] = abs
			out["candidates"] = []string{abs}
			return json.Marshal(out)
		}
	}
	base := filepath.Base(filepath.Clean(raw))
	cwd, err := os.Getwd()
	if err != nil {
		return json.Marshal(out)
	}
	type scope struct {
		root  string
		depth int
	}
	scopes := []scope{{cwd, 3}}
	up := cwd
	for i := 0; i < 3; i++ {
		parent := filepath.Dir(up)
		if parent == up {
			break
		}
		scopes = append(scopes, scope{parent, 1})
		up = parent
	}
	visited := 0
	seen := map[string]bool{}
	var found []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if visited > workspaceResolveMaxDirs || len(found) > 32 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || skipWalkDirs[e.Name()] {
				continue
			}
			visited++
			full := filepath.Join(dir, e.Name())
			if e.Name() == base && !seen[full] {
				seen[full] = true
				found = append(found, full)
			}
			if depth > 1 {
				walk(full, depth-1)
			}
		}
	}
	for _, sc := range scopes {
		if filepath.Base(sc.root) == base && !seen[sc.root] {
			seen[sc.root] = true
			found = append(found, sc.root)
		}
		walk(sc.root, sc.depth)
	}
	sort.Strings(found)
	if len(found) == 1 {
		out["path"] = found[0]
	}
	out["candidates"] = found
	return json.Marshal(out)
}

func handleReadFile(args json.RawMessage, workspace string) (string, error) {
	var in struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	path, err := resolvePath(workspace, in.Path)
	if err != nil {
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

	f, err := os.Open(path)
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

	if lineNo >= in.Offset+in.Limit {
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

func handleWriteFile(args json.RawMessage, workspace string) (string, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	path, err := resolvePath(workspace, in.Path)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), path), nil
}

func handleEditFile(args json.RawMessage, workspace string) (string, error) {
	var in struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: err.Error()}
	}
	path, err := resolvePath(workspace, in.Path)
	if err != nil {
		return "", err
	}
	if err := requireField("old_string", in.OldString); err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}

	content := string(data)
	count := strings.Count(content, in.OldString)
	if count == 0 {
		return "", &protocol.FrameError{Code: "edit_failed", Message: "old_string not found in " + path}
	}
	if count > 1 {
		return "", &protocol.FrameError{Code: "edit_failed", Message: fmt.Sprintf("old_string found %d times in %s; must be unique", count, path)}
	}

	newContent := strings.Replace(content, in.OldString, in.NewString, 1)
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return "", &protocol.FrameError{Code: "file_error", Message: err.Error()}
	}
	return fmt.Sprintf("edited %s", path), nil
}

func handleGrep(args json.RawMessage, workspace string) (string, error) {
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
	root := in.Path
	if root == "" {
		root = "."
	}
	rootPath, err := resolvePath(workspace, root)
	if err != nil {
		return "", err
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return "", &protocol.FrameError{Code: "bad_arguments", Message: "invalid regex: " + err.Error()}
	}

	var results []string
	truncated := false

	err = filepath.WalkDir(rootPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if in.Glob != "" {
			matched, err := filepath.Match(in.Glob, name)
			if err != nil || !matched {
				return nil
			}
		}
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

func handleGlob(args json.RawMessage, workspace string) (string, error) {
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
	root := in.Path
	if root == "" {
		root = "."
	}
	rootPath, err := resolvePath(workspace, root)
	if err != nil {
		return "", err
	}

	var matches []string
	if strings.Contains(in.Pattern, "**") {
		parts := strings.SplitN(in.Pattern, "**", 2)
		suffix := strings.TrimPrefix(parts[1], "/")
		if suffix == "" {
			suffix = "*"
		}

		err := filepath.WalkDir(rootPath, func(p string, d os.DirEntry, err error) error {
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
		pattern := filepath.Join(rootPath, in.Pattern)
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
