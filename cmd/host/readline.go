package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// readLineRaw reads one line with Tab completion against candidates(prefix).
// Falls back to a plain Scan when stdin is not a terminal.
func readLineRaw(prompt string, candidates func(prefix string) []string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		fmt.Print(prompt)
		return scanPlainLine()
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Print(prompt)
		return scanPlainLine()
	}
	defer func() { _ = term.Restore(fd, old) }()

	fmt.Print(prompt)
	var buf []rune
	var lastTab string
	tabStreak := 0

	for {
		var b [1]byte
		if _, err := os.Stdin.Read(b[:]); err != nil {
			fmt.Print("\r\n")
			return "", err
		}
		switch b[0] {
		case 3: // Ctrl+C
			fmt.Print("\r\n")
			return "", fmt.Errorf("interrupted")
		case 4: // Ctrl+D / EOF on empty
			if len(buf) == 0 {
				fmt.Print("\r\n")
				return "", nil
			}
		case '\r', '\n':
			fmt.Print("\r\n")
			return string(buf), nil
		case 127, 8: // backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				redrawLine(prompt, string(buf), "")
			}
			tabStreak = 0
		case '\t':
			line := string(buf)
			opts := candidates(line)
			if len(opts) == 0 {
				continue
			}
			if len(opts) == 1 {
				completed := opts[0]
				if strings.HasPrefix(completed, "/") && !strings.HasPrefix(line, "/") {
					// candidates always include leading slash for commands
				}
				// Store completed line without forcing a trailing space until next char.
				buf = []rune(trimToLine(completed, line))
				redrawLine(prompt, string(buf), "")
				tabStreak = 0
				lastTab = line
				continue
			}
			// Multiple candidates: complete common prefix, or list on second Tab.
			common := longestCommonPrefix(opts)
			if len(common) > len(strings.TrimPrefix(line, "")) && strings.HasPrefix(common, line) && common != line {
				buf = []rune(common)
				redrawLine(prompt, string(buf), "")
				tabStreak = 1
				lastTab = string(buf)
				continue
			}
			if tabStreak >= 1 && lastTab == line {
				redrawLine(prompt, string(buf), strings.Join(opts, "  "))
				tabStreak = 0
				continue
			}
			redrawLine(prompt, string(buf), strings.Join(opts, "  "))
			tabStreak = 1
			lastTab = line
		default:
			if b[0] < 32 {
				continue
			}
			// UTF-8 continuation
			r := rune(b[0])
			if b[0] >= 0x80 {
				var more []byte
				n := 1
				if b[0]&0xE0 == 0xC0 {
					n = 2
				} else if b[0]&0xF0 == 0xE0 {
					n = 3
				} else if b[0]&0xF8 == 0xF0 {
					n = 4
				}
				more = append(more, b[0])
				for len(more) < n {
					var c [1]byte
					if _, err := os.Stdin.Read(c[:]); err != nil {
						break
					}
					more = append(more, c[0])
				}
				rs := []rune(string(more))
				if len(rs) == 1 {
					r = rs[0]
				} else {
					continue
				}
			}
			buf = append(buf, r)
			redrawLine(prompt, string(buf), "")
			tabStreak = 0
		}
	}
}

// trimToLine turns a completion candidate into the new buffer content.
// Candidates for commands look like "/help"; plugin paths like "/llm-openai ".
func trimToLine(completed, _ string) string {
	return completed
}

func redrawLine(prompt, line, hint string) {
	// Clear from cursor start of line through a generous width, then rewrite.
	fmt.Print("\r\x1b[2K")
	fmt.Print(prompt + line)
	if hint != "" {
		fmt.Print("\r\n")
		fmt.Print("\x1b[2m" + hint + "\x1b[0m")
		fmt.Print("\r\n")
		fmt.Print(prompt + line)
	}
}

func longestCommonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	p := items[0]
	for _, s := range items[1:] {
		for !strings.HasPrefix(s, p) {
			if p == "" {
				return ""
			}
			p = p[:len(p)-1]
		}
	}
	return p
}

func scanPlainLine() (string, error) {
	var b strings.Builder
	var ch [1]byte
	for {
		n, err := os.Stdin.Read(ch[:])
		if n == 0 || err != nil {
			if b.Len() == 0 {
				return "", err
			}
			return b.String(), nil
		}
		if ch[0] == '\n' {
			return strings.TrimRight(b.String(), "\r"), nil
		}
		b.WriteByte(ch[0])
	}
}

// completeSlash returns completion candidates for a line that may start with /.
func (cp *commandPlane) completeSlash(line string) []string {
	if !strings.HasPrefix(line, "/") {
		return nil
	}
	body := line[1:]
	// /plugin subcommand
	if i := strings.IndexByte(body, ' '); i >= 0 {
		name := strings.ToLower(body[:i])
		m, ok := cp.manifests[name]
		if !ok {
			return nil
		}
		prefix := body[i+1:]
		var out []string
		for _, c := range m.Commands {
			cand := "/" + name + " " + c.Name
			if strings.HasPrefix(cand, "/"+name+" "+prefix) || prefix == "" {
				out = append(out, cand)
			}
		}
		return out
	}
	// /partial → native + plugin names
	native := []string{"/help", "/lp", "/refresh", "/dump-trace", "/exit"}
	var out []string
	for _, n := range native {
		if strings.HasPrefix(n, line) {
			out = append(out, n)
		}
	}
	for _, name := range cp.mounted {
		cand := "/" + name
		if strings.HasPrefix(cand, line) {
			out = append(out, cand)
		}
	}
	return out
}
