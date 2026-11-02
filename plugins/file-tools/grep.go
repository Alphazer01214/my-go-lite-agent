package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

type grepArgs struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path,omitempty"`
	Glob          string `json:"glob,omitempty"`
	Regex         bool   `json:"regex,omitempty"`
	CaseSensitive *bool  `json:"case_sensitive,omitempty"`
	MaxMatches    int    `json:"max_matches,omitempty"`
	MaxFiles      int    `json:"max_files,omitempty"`
	MaxFileBytes  int    `json:"max_file_bytes,omitempty"`
}

type grepMatcher struct {
	regex       *regexp.Regexp
	literal     string
	literalFold string
	fold        bool
}

func (m *grepMatcher) find(line string) int {
	if m.regex != nil {
		location := m.regex.FindStringIndex(line)
		if location == nil {
			return -1
		}
		return location[0]
	}
	if m.fold {
		return strings.Index(strings.ToLower(line), m.literalFold)
	}
	return strings.Index(line, m.literal)
}

func (p *plugin) grep(ctx context.Context, req *providerRequest, args *grepArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}
	if strings.TrimSpace(args.Pattern) == "" {
		return toolResult{}, badArguments("pattern is required")
	}
	if strings.ContainsRune(args.Pattern, 0) {
		return toolResult{}, badArguments("pattern contains NUL")
	}
	if args.MaxMatches == 0 {
		args.MaxMatches = defaultGrepMatch
	}
	if args.MaxMatches < 0 {
		return toolResult{}, badArguments("max_matches must not be negative")
	}
	if args.MaxMatches > maxGrepMatch {
		args.MaxMatches = maxGrepMatch
	}
	if args.MaxFiles == 0 {
		args.MaxFiles = defaultGrepFiles
	}
	if args.MaxFiles < 0 {
		return toolResult{}, badArguments("max_files must not be negative")
	}
	if args.MaxFiles > maxGrepFiles {
		args.MaxFiles = maxGrepFiles
	}
	if args.MaxFileBytes == 0 {
		args.MaxFileBytes = maxGrepFileBytes
	}
	if args.MaxFileBytes < 0 {
		return toolResult{}, badArguments("max_file_bytes must not be negative")
	}
	if args.MaxFileBytes > maxGrepFileBytes {
		args.MaxFileBytes = maxGrepFileBytes
	}
	caseSensitive := true
	if args.CaseSensitive != nil {
		caseSensitive = *args.CaseSensitive
	}

	matcher, err := buildGrepMatcher(args.Pattern, args.Regex, caseSensitive)
	if err != nil {
		return toolResult{}, err
	}
	var globFilter *regexp.Regexp
	if strings.TrimSpace(args.Glob) != "" {
		globFilter, err = globMatcher(args.Glob)
		if err != nil {
			return toolResult{}, err
		}
	}

	workspace, err := resolveWorkspace(req.Workspace)
	if err != nil {
		return toolResult{}, err
	}
	searchPath := strings.TrimSpace(args.Path)
	if searchPath == "" {
		searchPath = "."
	}
	path, err := resolvePath(workspace, searchPath)
	if err != nil {
		return toolResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fileError("grep", searchPath, err)
	}

	var lines []string
	filesScanned := 0
	truncated := false
	outputBytes := 0

	searchFile := func(filePath string) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		rel, err := filepath.Rel(workspace, filePath)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if globFilter != nil && !globFilter.MatchString(rel) {
			return nil
		}
		fileInfo, err := os.Stat(filePath)
		if err != nil || fileInfo.IsDir() || fileInfo.Size() > int64(args.MaxFileBytes) {
			return nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		if strings.IndexByte(string(data), 0) >= 0 || !utf8.Valid(data) {
			return nil
		}
		filesScanned++
		for lineIndex, line := range strings.Split(string(data), "\n") {
			if err := checkContext(ctx); err != nil {
				return err
			}
			line = strings.TrimSuffix(line, "\r")
			column := matcher.find(line)
			if column < 0 {
				continue
			}
			entry := fmt.Sprintf("%s:%d:%d: %s", rel, lineIndex+1, column+1, line)
			if outputBytes+len(entry)+1 > maxGrepOutputSize {
				truncated = true
				return fs.SkipAll
			}
			lines = append(lines, entry)
			outputBytes += len(entry) + 1
			if len(lines) >= args.MaxMatches {
				truncated = true
				return fs.SkipAll
			}
		}
		if filesScanned >= args.MaxFiles {
			truncated = true
			return fs.SkipAll
		}
		return nil
	}

	if !info.IsDir() {
		if err := searchFile(path); err != nil && !errors.Is(err, fs.SkipAll) {
			return toolResult{}, err
		}
	} else {
		walkErr := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			return searchFile(current)
		})
		if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
			if ctx.Err() != nil {
				return toolResult{}, checkContext(ctx)
			}
			return fileError("grep", searchPath, walkErr)
		}
	}

	meta := map[string]any{
		"pattern":       args.Pattern,
		"path":          searchPath,
		"files_scanned": filesScanned,
		"matches":       len(lines),
		"truncated":     truncated,
	}
	result := success(strings.Join(lines, "\n"), meta)
	result.Truncated = truncated
	return result, nil
}

func buildGrepMatcher(pattern string, regex bool, caseSensitive bool) (*grepMatcher, error) {
	if regex {
		expression := pattern
		if !caseSensitive {
			expression = "(?i)" + expression
		}
		compiled, err := regexp.Compile(expression)
		if err != nil {
			return nil, badArguments("invalid regular expression: %v", err)
		}
		return &grepMatcher{regex: compiled}, nil
	}
	matcher := &grepMatcher{literal: pattern}
	if !caseSensitive {
		matcher.fold = true
		matcher.literalFold = strings.ToLower(pattern)
	}
	return matcher, nil
}
