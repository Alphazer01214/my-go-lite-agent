package utils

import "strings"

func NormalizePath(path string) string {
	for _, sep := range []string{"/", "\\"} {
		path = strings.ReplaceAll(path, sep, "/")
	}
	return path
}
