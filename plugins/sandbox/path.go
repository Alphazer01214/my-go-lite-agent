package main

import "path/filepath"

func filepathJoin(parts ...string) string {
	return filepath.Join(parts...)
}
