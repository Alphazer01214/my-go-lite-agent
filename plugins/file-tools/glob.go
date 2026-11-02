package main

import (
	"context"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type globArgs struct {
	Pattern    string `json:"pattern"`
	MaxResults int    `json:"max_results,omitempty"`
}

func (p *plugin) glob(ctx context.Context, req *providerRequest, args *globArgs) (toolResult, error) {
	if err := checkContext(ctx); err != nil {
		return toolResult{}, err
	}
	pattern, err := normalizeGlobPattern(args.Pattern)
	if err != nil {
		return toolResult{}, err
	}
	if args.MaxResults == 0 {
		args.MaxResults = defaultGlobLimit
	}
	if args.MaxResults < 0 {
		return toolResult{}, badArguments("max_results must not be negative")
	}
	if args.MaxResults > maxGlobLimit {
		args.MaxResults = maxGlobLimit
	}
	matcher, err := compileGlob(pattern)
	if err != nil {
		return toolResult{}, badArguments("invalid glob pattern: %v", err)
	}

	workspace, err := resolveWorkspace(req.Workspace)
	if err != nil {
		return toolResult{}, err
	}

	results := make([]string, 0, args.MaxResults)
	truncated := false
	walkErr := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, err error) error {
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
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !matcher.MatchString(rel) {
			return nil
		}
		results = append(results, rel)
		if len(results) >= args.MaxResults {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		if ctx.Err() != nil {
			return toolResult{}, checkContext(ctx)
		}
		return fileError("glob", pattern, walkErr)
	}
	sort.Strings(results)

	meta := map[string]any{
		"pattern":   pattern,
		"count":     len(results),
		"truncated": truncated,
	}
	result := success(strings.Join(results, "\n"), meta)
	result.Truncated = truncated
	return result, nil
}

func normalizeGlobPattern(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", badArguments("pattern is required")
	}
	if strings.ContainsRune(raw, 0) {
		return "", badArguments("pattern contains NUL")
	}
	pattern := filepath.ToSlash(raw)
	if pathpkg.IsAbs(pattern) || filepath.VolumeName(raw) != "" {
		return "", badArguments("pattern must be relative to the workspace")
	}
	pattern = pathpkg.Clean(pattern)
	if pattern == "." {
		return "", badArguments("pattern must select files")
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == ".." {
			return "", badArguments("pattern must not contain ..")
		}
	}
	return pattern, nil
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(pattern); {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				if index+2 < len(pattern) && pattern[index+2] == '/' {
					builder.WriteString("(?:.*/)?")
					index += 3
				} else {
					builder.WriteString(".*")
					index += 2
				}
				continue
			}
			builder.WriteString("[^/]*")
			index++
		case '?':
			builder.WriteString("[^/]")
			index++
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[index])))
			index++
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func globMatcher(pattern string) (*regexp.Regexp, error) {
	normalized, err := normalizeGlobPattern(pattern)
	if err != nil {
		return nil, err
	}
	return compileGlob(normalized)
}
