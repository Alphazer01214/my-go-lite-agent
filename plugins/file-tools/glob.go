package main

import (
	"context"
	"encoding/json"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type globIn struct {
	Pattern    string `json:"pattern"`
	MaxResults *int   `json:"max_results,omitempty"`
}

func (p *plugin) glob(_ context.Context, _ *pluginsdk.Request, input ProviderInput, arguments json.RawMessage) (json.RawMessage, error) {
	var in globIn
	if err := DecodeStrict(arguments, &in); err != nil {
		return nil, err
	}
	if err := validateGlobPattern(in.Pattern); err != nil {
		return nil, err
	}
	w, err := openWorkspace(input.Workspace)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	files, _, err := walkFiles(w, ".", 10000)
	if err != nil {
		return nil, mapPathError(err)
	}
	limit := 200
	if in.MaxResults != nil {
		limit = *in.MaxResults
	}
	var matches []string
	for _, file := range files {
		if matchGlob(in.Pattern, file) {
			matches = append(matches, file)
			if len(matches) > limit {
				break
			}
		}
	}
	truncated := len(matches) > limit
	if truncated {
		matches = matches[:limit]
	}
	sort.Strings(matches)
	return Encode(Result{
		Content: strings.Join(matches, "\n"), Truncated: truncated,
		Meta: map[string]any{"pattern": in.Pattern, "paths": matches},
	})
}

func validateGlobPattern(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "pattern is required")
	}
	normalized := strings.ReplaceAll(pattern, `\`, "/")
	if strings.ContainsRune(normalized, 0) || strings.HasPrefix(normalized, "/") ||
		filepath.IsAbs(pattern) || filepath.VolumeName(pattern) != "" {
		return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "pattern must be relative")
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == "" || part == "." || part == ".." {
			return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "pattern contains an invalid component")
		}
	}
	return nil
}

func walkFiles(w *workspace, base string, maxFiles int) ([]string, bool, error) {
	var out []string
	var walk func(string) error
	walk = func(dir string) error {
		file, err := w.root.Open(dir)
		if err != nil {
			return err
		}
		items, err := file.ReadDir(-1)
		_ = file.Close()
		if err != nil {
			return err
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })
		for _, item := range items {
			child := joinRelative(dir, item.Name())
			info, err := item.Info()
			if err != nil {
				return err
			}
			if info.IsDir() {
				if err := walk(filepath.FromSlash(child)); err != nil {
					return err
				}
				continue
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				continue
			}
			if info.Mode().IsRegular() {
				out = append(out, child)
				if len(out) >= maxFiles {
					return nil
				}
			}
		}
		return nil
	}
	err := walk(base)
	truncated := len(out) >= maxFiles
	sort.Strings(out)
	return out, truncated, err
}

func matchGlob(pattern, name string) bool {
	pattern = strings.ReplaceAll(pattern, `\`, "/")
	name = displayPath(name)
	return matchGlobParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchGlobParts(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		if matchGlobParts(pattern[1:], name) {
			return true
		}
		return len(name) > 0 && matchGlobParts(pattern, name[1:])
	}
	if len(name) == 0 || !matchSegment(pattern[0], name[0]) {
		return false
	}
	return matchGlobParts(pattern[1:], name[1:])
}

func matchSegment(pattern, value string) bool {
	p := []rune(pattern)
	v := []rune(value)
	dp := make([][]bool, len(p)+1)
	for i := range dp {
		dp[i] = make([]bool, len(v)+1)
	}
	dp[0][0] = true
	for i := 1; i <= len(p); i++ {
		if p[i-1] == '*' {
			dp[i][0] = dp[i-1][0]
		}
		for j := 1; j <= len(v); j++ {
			switch p[i-1] {
			case '*':
				dp[i][j] = dp[i-1][j] || dp[i][j-1]
			case '?':
				dp[i][j] = dp[i-1][j-1] && v[j-1] != '/'
			default:
				dp[i][j] = dp[i-1][j-1] && p[i-1] == v[j-1]
			}
		}
	}
	return dp[len(p)][len(v)]
}

func isText(data []byte) bool {
	return len(data) == 0 || (utf8.Valid(data) && !strings.ContainsRune(string(data), 0))
}
