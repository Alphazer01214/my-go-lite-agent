package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const maxReadFileBytes = 16 << 20

type readIn struct {
	Path      string `json:"path"`
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
	MaxBytes  *int   `json:"max_bytes,omitempty"`
}

func (p *plugin) read(_ context.Context, _ *pluginsdk.Request, input ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in readIn
	if err := DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	w, err := openWorkspace(input.Workspace)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	rel, err := safeRelative(in.Path, false)
	if err != nil {
		return nil, err
	}
	info, err := w.root.Stat(rel)
	if err != nil {
		return nil, mapPathError(err)
	}
	if info.IsDir() {
		return nil, pluginsdk.ErrCode("path_is_directory", "path is a directory")
	}
	if info.Size() > maxReadFileBytes {
		return nil, pluginsdk.ErrCode("file_too_large", "file exceeds 16 MiB")
	}
	data, err := w.root.ReadFile(rel)
	if err != nil {
		return nil, mapPathError(err)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return Failure("file_binary", "file contains NUL bytes", map[string]any{"path": displayPath(rel)})
	}
	if !utf8.Valid(data) {
		return Failure("file_binary", "file is not valid UTF-8", map[string]any{"path": displayPath(rel)})
	}

	lines := splitLines(string(data))
	start := 1
	if in.StartLine != nil {
		start = *in.StartLine
	}
	end := len(lines)
	if in.EndLine != nil {
		end = *in.EndLine
	}
	if start < 1 || end < start || start > len(lines) {
		return nil, pluginsdk.ErrCode("line_range_invalid", "line range is outside the file")
	}
	if end > len(lines) {
		end = len(lines)
	}
	selected := strings.Join(lines[start-1:end], "\n")
	limit := 64 << 10
	if in.MaxBytes != nil {
		limit = *in.MaxBytes
	}
	content, truncated := truncateUTF8(selected, limit)
	meta := map[string]any{
		"path":       displayPath(rel),
		"size":       len(data),
		"line_start": start,
		"line_end":   end,
		"line_count": len(lines),
		"encoding":   "utf-8",
	}
	if len(data) <= 8<<20 {
		meta["sha256"] = hashBytes(data)
	}
	return Encode(Result{Content: content, Truncated: truncated, Meta: meta})
}

func splitLines(value string) []string {
	if value == "" {
		return []string{""}
	}
	lines := strings.Split(value, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
