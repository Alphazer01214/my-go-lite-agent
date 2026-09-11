// Package mdansi renders Markdown to ANSI for terminal UIs.
// Parsing is goldmark (CommonMark); ANSI styling stays in this package.
package mdansi

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
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

var parser = goldmark.New(goldmark.WithExtensions(extension.Table))

// Render converts Markdown to ANSI-styled plain text (trailing newline included).
func Render(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	reader := text.NewReader([]byte(src))
	doc := parser.Parser().Parse(reader)
	var b strings.Builder
	renderNode(&b, doc, []byte(src), 0)
	out := b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(out, "\n") + "\n"
}

func renderNode(b *strings.Builder, n ast.Node, source []byte, listDepth int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch node := c.(type) {
		case *ast.Heading:
			renderHeading(b, node, source)
		case *ast.Paragraph:
			if isInTightList(node) {
				renderInline(b, node, source)
			} else {
				renderInline(b, node, source)
				b.WriteString("\n")
			}
		case *ast.TextBlock:
			renderInline(b, node, source)
			b.WriteString("\n")
		case *ast.FencedCodeBlock:
			renderFenced(b, node, source)
		case *ast.CodeBlock:
			renderIndentedCode(b, node, source)
		case *ast.Blockquote:
			renderBlockquote(b, node, source)
		case *ast.List:
			renderList(b, node, source, listDepth)
		case *ast.ThematicBreak:
			b.WriteString(ansiGray + "────────────────" + ansiReset + "\n")
		case *extensionast.Table:
			renderTable(b, node, source)
		case *ast.HTMLBlock:
			// Skip raw HTML in terminal output.
		case *ast.Text:
			writeText(b, string(node.Segment.Value(source)))
			if node.SoftLineBreak() {
				b.WriteString("\n")
			}
		default:
			renderNode(b, c, source, listDepth)
		}
	}
}

func isInTightList(p ast.Node) bool {
	parent := p.Parent()
	if parent == nil {
		return false
	}
	list, ok := parent.(*ast.List)
	if !ok {
		return false
	}
	return list.IsTight
}

func renderHeading(b *strings.Builder, h *ast.Heading, source []byte) {
	color := ansiCyan
	if h.Level == 1 {
		color = ansiBold + ansiCyan
	}
	b.WriteString(color)
	renderInline(b, h, source)
	b.WriteString(ansiReset + "\n")
}

func renderTable(b *strings.Builder, table *extensionast.Table, source []byte) {
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		switch tr := row.(type) {
		case *extensionast.TableHeader:
			writeTableRow(b, tr, source)
		case *extensionast.TableRow:
			writeTableRow(b, tr, source)
		}
	}
	b.WriteString("\n")
}

func writeTableRow(b *strings.Builder, row interface{ FirstChild() ast.Node }, source []byte) {
	var cells []string
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		tc, ok := cell.(*extensionast.TableCell)
		if !ok {
			continue
		}
		var cb strings.Builder
		renderInline(&cb, tc, source)
		cells = append(cells, strings.TrimSpace(strings.ReplaceAll(cb.String(), "\n", " ")))
	}
	if len(cells) == 0 {
		return
	}
	b.WriteString("│ " + strings.Join(cells, " │ ") + " │\n")
}

func renderBlockquote(b *strings.Builder, q *ast.Blockquote, source []byte) {
	var inner strings.Builder
	renderNode(&inner, q, source, 0)
	for _, ln := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
		if ln == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(ansiGray + "│ " + ansiReset + ln + "\n")
	}
}

func renderList(b *strings.Builder, list *ast.List, source []byte, depth int) {
	i := list.Start
	if i == 0 {
		i = 1
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		bullet := "•"
		if list.IsOrdered() {
			bullet = fmt.Sprintf("%d.", i)
			i++
		}
		pad := strings.Repeat("  ", depth)
		b.WriteString(ansiGreen + pad + bullet + ansiReset + " ")
		var inner strings.Builder
		renderNode(&inner, li, source, depth+1)
		lines := strings.Split(strings.TrimRight(inner.String(), "\n"), "\n")
		for idx, ln := range lines {
			if idx == 0 {
				b.WriteString(ln + "\n")
				continue
			}
			if ln == "" {
				b.WriteString("\n")
				continue
			}
			b.WriteString(pad + "  " + ln + "\n")
		}
	}
	b.WriteString("\n")
}

func renderFenced(b *strings.Builder, fc *ast.FencedCodeBlock, source []byte) {
	lang := string(fc.Language(source))
	label := lang
	if label == "" {
		label = "code"
	}
	b.WriteString(ansiDim + "┌─ " + label + ansiReset + "\n")
	lines := linesFromSegments(fc.Lines(), source)
	for _, ln := range lines {
		b.WriteString(ansiDim + "│ " + ansiReset + ln + "\n")
	}
	b.WriteString(ansiDim + "└─" + ansiReset + "\n")
}

func renderIndentedCode(b *strings.Builder, cb *ast.CodeBlock, source []byte) {
	b.WriteString(ansiDim + "┌─ code" + ansiReset + "\n")
	for _, ln := range linesFromSegments(cb.Lines(), source) {
		b.WriteString(ansiDim + "│ " + ansiReset + ln + "\n")
	}
	b.WriteString(ansiDim + "└─" + ansiReset + "\n")
}

func linesFromSegments(segs *text.Segments, source []byte) []string {
	var out []string
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		raw := string(seg.Value(source))
		raw = strings.TrimRight(raw, "\n")
		out = append(out, raw)
	}
	return out
}

func renderInline(b *strings.Builder, n ast.Node, source []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch node := c.(type) {
		case *ast.Text:
			writeText(b, string(node.Segment.Value(source)))
			if node.SoftLineBreak() {
				b.WriteString("\n")
			}
		case *ast.String:
			writeText(b, string(node.Value))
		case *ast.CodeSpan:
			b.WriteString(ansiYellow + string(node.Text(source)) + ansiReset)
		case *ast.Emphasis:
			open, closeTag := ansiItalic, ansiReset
			if node.Level >= 2 {
				open = ansiBold
			}
			b.WriteString(open)
			renderInline(b, node, source)
			b.WriteString(closeTag)
		case *ast.Link:
			renderInline(b, node, source)
			dest := string(node.Destination)
			if dest != "" {
				b.WriteString(ansiGray + " (" + dest + ")" + ansiReset)
			}
		case *ast.AutoLink:
			b.WriteString(ansiCyan + string(node.URL(source)) + ansiReset)
		case *ast.Image:
			b.WriteString("[image]")
		case *ast.RawHTML:
			// skip
		default:
			renderInline(b, c, source)
		}
	}
}

func writeText(b *strings.Builder, s string) {
	b.WriteString(s)
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
