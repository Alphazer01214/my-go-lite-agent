package serve

import (
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// Render kinds (protocol v2). See CONTEXT.md Presentation.
const (
	KindMarkdownText = string(pluginsdk.RenderMarkdownText)
	KindMessageText  = string(pluginsdk.RenderMessageText)
	KindSummaryText  = string(pluginsdk.RenderSummaryText)
)

// Truncation limit for summary_text detail display (rune-counted).
const MaxSummaryDetail = 400

// SummaryPair / RenderIntent / PanelOp are Host aliases of the pluginsdk wire
// contract: one definition, shared by Host and Plugins (ADR-0026 — the render
// shape is a public contract, Host does not keep a private copy).
type (
	SummaryPair  = pluginsdk.SummaryPair
	RenderIntent = pluginsdk.RenderIntent
	PanelOp      = pluginsdk.PanelOp
)

// TruncateRunes shortens s to max runes, appending … when cut.
func TruncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}