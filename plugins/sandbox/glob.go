package main

import (
	"path/filepath"
	"runtime"
	"strings"
)

// matchPath matches a path against a glob pattern.
//
// Both sides are resolved against the workspace when they are relative, then
// cleaned and normalized to slash separators. The pattern supports "*" (any
// run of characters inside one segment), "**" (any run of segments) and "?"
// (one character inside one segment).
func matchPath(pattern, path, workspace string) bool {
	if pattern == "" || path == "" {
		return false
	}
	resolvedPattern := resolvePath(pattern, workspace)
	resolvedPath := resolvePath(path, workspace)
	if resolvedPattern == "" || resolvedPath == "" {
		return false
	}
	return matchSegments(strings.Split(resolvedPattern, "/"), strings.Split(resolvedPath, "/"))
}

// resolvePath turns a path or a pattern into a comparable absolute form.
func resolvePath(value, workspace string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !filepath.IsAbs(value) && workspace != "" {
		value = filepath.Join(workspace, value)
	}
	value = filepath.Clean(value)
	value = filepath.ToSlash(value)
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}

// matchSegments matches a segmented pattern against a segmented path.
func matchSegments(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		// "**" matches zero or more whole segments.
		for i := 0; i <= len(path); i++ {
			if matchSegments(pattern[1:], path[i:]) {
				return true
			}
		}
		return false
	}
	if len(path) == 0 {
		return false
	}
	if !matchSegment(pattern[0], path[0]) {
		return false
	}
	return matchSegments(pattern[1:], path[1:])
}

// matchSegment matches one path segment against one pattern segment.
func matchSegment(pattern, value string) bool {
	p, v := 0, 0
	star, resume := -1, 0
	for v < len(value) {
		switch {
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == value[v]):
			p++
			v++
		case p < len(pattern) && pattern[p] == '*':
			star = p
			resume = v
			p++
		case star != -1:
			p = star + 1
			resume++
			v = resume
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

// matchCommand matches a command line against a pattern.
//
// The pattern is a whitespace separated token list. Every token must match the
// command token at the same position, and the pattern may stop early: it is a
// prefix match, so "rm -rf *" covers "rm -rf build" and "rm -rf build/x".
func matchCommand(pattern, command string) bool {
	patternTokens := strings.Fields(pattern)
	commandTokens := strings.Fields(command)
	if len(patternTokens) == 0 || len(patternTokens) > len(commandTokens) {
		return false
	}
	for i, token := range patternTokens {
		if !matchToken(token, commandTokens[i]) {
			return false
		}
	}
	return true
}

// matchToken matches one whitespace separated command token.
func matchToken(pattern, value string) bool {
	p, v := 0, 0
	star, resume := -1, 0
	for v < len(value) {
		switch {
		case p < len(pattern) && (pattern[p] == '?' || pattern[p] == value[v]):
			p++
			v++
		case p < len(pattern) && pattern[p] == '*':
			star = p
			resume = v
			p++
		case star != -1:
			p = star + 1
			resume++
			v = resume
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
