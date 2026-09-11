// Package mdansi renders a practical Markdown subset to ANSI for terminal UIs.
// Stdlib-only; used by the Host CLI Render Medium.
package mdansi

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiItalic = "\x1b[3m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiGreen  = "\x1b[32m"
	ansiGray   = "\x1b[90m"
)

var (
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic = regexp.MustCompile(`\*([^*]+)\*`)
	reCode   = regexp.MustCompile("`([^`]+)`")
)

// Render converts Markdown to ANSI-styled plain text (trailing newline included).
func Render(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")
	var b strings.Builder
	inFence := false
	fenceLang := ""
	var fence []string
	inList := false

	flushList := func() {
		if inList {
			b.WriteString("\n")
			inList = false
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inFence {
				flushList()
				inFence = true
				fenceLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				fence = nil
				continue
			}
			b.WriteString(renderFence(fenceLang, fence))
			inFence = false
			continue
		}
		if inFence {
			fence = append(fence, line)
			continue
		}

		if trimmed == "" {
			flushList()
			b.WriteString("\n")
			continue
		}
		if trimmed == "---" || trimmed == "***" {
			flushList()
			b.WriteString(ansiGray + "────────────────" + ansiReset + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			flushList()
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			color := ansiCyan
			if level == 1 {
				color = ansiBold + ansiCyan
			}
			b.WriteString(color + inline(text) + ansiReset + "\n")
			continue
		}
		if strings.HasPrefix(trimmed, ">") {
			flushList()
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			b.WriteString(ansiGray + "│ " + ansiReset + inline(text) + "\n")
			continue
		}
		if m := listBullet(trimmed); m != "" {
			inList = true
			item := stripListPrefix(trimmed)
			b.WriteString(ansiGreen + m + ansiReset + " " + inline(item) + "\n")
			continue
		}
		flushList()
		b.WriteString(inline(trimmed) + "\n")
	}
	if inFence {
		b.WriteString(renderFence(fenceLang, fence))
	}
	out := b.String()
	// Collapse 3+ blank lines.
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(out, "\n") + "\n"
}

func listBullet(trimmed string) string {
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return "•"
	}
	// "1. " style
	if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' {
		j := 0
		for j < len(trimmed) && trimmed[j] >= '0' && trimmed[j] <= '9' {
			j++
		}
		if j < len(trimmed)-1 && trimmed[j] == '.' && trimmed[j+1] == ' ' {
			return trimmed[:j+1]
		}
	}
	return ""
}

func stripListPrefix(trimmed string) string {
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return strings.TrimSpace(trimmed[2:])
	}
	if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' {
		j := 0
		for j < len(trimmed) && trimmed[j] >= '0' && trimmed[j] <= '9' {
			j++
		}
		if j < len(trimmed)-1 && trimmed[j] == '.' && trimmed[j+1] == ' ' {
			return strings.TrimSpace(trimmed[j+2:])
		}
	}
	return strings.TrimSpace(trimmed)
}

func renderFence(lang string, lines []string) string {
	var b strings.Builder
	label := lang
	if label == "" {
		label = "code"
	}
	b.WriteString(ansiDim + "┌─ " + label + ansiReset + "\n")
	for _, ln := range lines {
		b.WriteString(ansiDim + "│ " + ansiReset + ln + "\n")
	}
	b.WriteString(ansiDim + "└─" + ansiReset + "\n")
	return b.String()
}

func inline(s string) string {
	s = reCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := reCode.ReplaceAllString(m, "$1")
		return ansiYellow + inner + ansiReset
	})
	s = reBold.ReplaceAllStringFunc(s, func(m string) string {
		inner := reBold.ReplaceAllString(m, "$1")
		return ansiBold + inner + ansiReset
	})
	s = reItalic.ReplaceAllStringFunc(s, func(m string) string {
		inner := reItalic.ReplaceAllString(m, "$1")
		return ansiItalic + inner + ansiReset
	})
	return s
}

// Plain strips ANSI codes (for width tests / logging).
func Plain(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Indent prefixes each non-empty line with pad.
func Indent(s, pad string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		if ln == "" {
			continue
		}
		lines[i] = pad + ln
	}
	return strings.Join(lines, "\n") + "\n"
}

// TitleLine formats a short title for expandable sections.
func TitleLine(icon, title string) string {
	return fmt.Sprintf("%s %s", icon, title)
}
