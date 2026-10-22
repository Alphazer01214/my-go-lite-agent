package main

import (
	"context"
	"encoding/json"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type searchIn struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path,omitempty"`
	Glob          string `json:"glob,omitempty"`
	Regex         *bool  `json:"regex,omitempty"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"`
	MaxMatches    *int   `json:"max_matches,omitempty"`
	MaxFiles      *int   `json:"max_files,omitempty"`
	MaxFileBytes  *int   `json:"max_file_bytes,omitempty"`
	ContextLines  *int   `json:"context_lines,omitempty"`
}

type matchOut struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

func (p *plugin) search(_ context.Context, _ *pluginsdk.Request, input ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in searchIn
	if err := DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	if in.Pattern == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "pattern is required")
	}
	if in.Path == "" {
		in.Path = "."
	}
	if in.Glob != "" {
		if err := validateGlobPattern(in.Glob); err != nil {
			return nil, err
		}
	}
	maxMatches := intDefault(in.MaxMatches, 200)
	maxFiles := intDefault(in.MaxFiles, 2000)
	maxFileBytes := intDefault(in.MaxFileBytes, 1<<20)
	contextLines := intDefault(in.ContextLines, 0)
	useRegex := in.Regex != nil && *in.Regex
	caseSensitive := in.CaseSensitive == nil || *in.CaseSensitive

	var re *regexp.Regexp
	var err error
	if useRegex {
		pattern := in.Pattern
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err = regexp.Compile(pattern)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "invalid regex: "+err.Error())
		}
	}
	w, err := openWorkspace(input.Workspace)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	base, err := safeRelative(in.Path, true)
	if err != nil {
		return nil, err
	}
	info, err := w.root.Stat(base)
	if err != nil {
		return nil, mapPathError(err)
	}
	var files []string
	if info.IsDir() {
		files, _, err = walkFiles(w, base, maxFiles)
		if err != nil {
			return nil, mapPathError(err)
		}
	} else {
		files = []string{displayPath(base)}
	}
	var matches []matchOut
	truncated := false
	for _, file := range files {
		if in.Glob != "" && !matchGlob(in.Glob, file) {
			continue
		}
		fi, err := w.root.Stat(filepath.FromSlash(file))
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > int64(maxFileBytes) {
			continue
		}
		data, err := w.root.ReadFile(filepath.FromSlash(file))
		if err != nil || !isText(data) {
			continue
		}
		lines := splitLines(string(data))
		for lineIndex, line := range lines {
			columns := matchColumns(line, in.Pattern, caseSensitive, re)
			for _, column := range columns {
				matches = append(matches, matchOut{
					Path: file, Line: lineIndex + 1, Column: column, Text: line,
				})
				if len(matches) >= maxMatches {
					truncated = true
					break
				}
			}
			if truncated {
				break
			}
		}
		if truncated {
			break
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		if matches[i].Line != matches[j].Line {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Column < matches[j].Column
	})
	content := formatSearchContent(w, files, matches, contextLines, maxFileBytes)
	return Encode(Result{
		Content: content, Truncated: truncated,
		Meta: map[string]any{"matches": matches, "files_scanned": len(files)},
	})
}

func matchColumns(line, literal string, caseSensitive bool, re *regexp.Regexp) []int {
	if re != nil {
		indexes := re.FindAllStringIndex(line, -1)
		out := make([]int, 0, len(indexes))
		for _, index := range indexes {
			out = append(out, utf8.RuneCountInString(line[:index[0]])+1)
		}
		return out
	}
	searchLine, searchLiteral := line, literal
	if !caseSensitive {
		searchLine = strings.ToLower(searchLine)
		searchLiteral = strings.ToLower(searchLiteral)
	}
	var out []int
	offset := 0
	for {
		index := strings.Index(searchLine[offset:], searchLiteral)
		if index < 0 {
			break
		}
		absolute := offset + index
		out = append(out, utf8.RuneCountInString(line[:absolute])+1)
		offset = absolute + len(searchLiteral)
		if offset >= len(searchLine) {
			break
		}
	}
	return out
}

func formatSearchContent(w *workspace, files []string, matches []matchOut, contextLines, maxFileBytes int) string {
	if len(matches) == 0 {
		return ""
	}
	fileSet := make(map[string]bool, len(files))
	for _, file := range files {
		fileSet[file] = true
	}
	var builder strings.Builder
	for _, match := range matches {
		if !fileSet[match.Path] {
			continue
		}
		fi, err := w.root.Stat(filepath.FromSlash(match.Path))
		if err != nil || fi.Size() > int64(maxFileBytes) {
			continue
		}
		data, err := w.root.ReadFile(filepath.FromSlash(match.Path))
		if err != nil {
			continue
		}
		lines := splitLines(string(data))
		start := match.Line - contextLines
		if start < 1 {
			start = 1
		}
		end := match.Line + contextLines
		if end > len(lines) {
			end = len(lines)
		}
		for lineNo := start; lineNo <= end; lineNo++ {
			builder.WriteString(match.Path)
			builder.WriteByte(':')
			builder.WriteString(formatInt(int64(lineNo)))
			builder.WriteByte(':')
			if lineNo == match.Line {
				builder.WriteString(formatInt(int64(match.Column)))
			} else {
				builder.WriteByte('0')
			}
			builder.WriteByte(':')
			builder.WriteString(lines[lineNo-1])
			builder.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(builder.String(), "\n")
}
