package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func humanContent(value any) string {
	body, _ := json.MarshalIndent(value, "", "  ")
	return string(body)
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func displayPath(value string) string {
	value = strings.ReplaceAll(value, `\`, "/")
	value = strings.TrimPrefix(value, "./")
	if value == "" {
		return "."
	}
	return value
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func joinRelative(base, name string) string {
	if base == "." || base == "" {
		return name
	}
	return path.Join(displayPath(base), name)
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func intDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
