package serve

import (
	"unicode/utf8"
)

// Render kinds (protocol v2). See CONTEXT.md Presentation.
const (
	KindMarkdownText = "markdown_text"
	KindMessageText  = "message_text"
	KindSummaryText  = "summary_text"
)

// Truncation limit for summary_text detail display (rune-counted).
const MaxSummaryDetail = 400

// SummaryPair is one ordered key/value row on a summary_text card.
type SummaryPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RenderIntent is one classified main-window block for the Render Medium.
// JSON field names match pluginsdk so Web SSE and Frame payloads share one wire shape.
// SessionID scopes host-loop intents so multi-session Web shells ignore foreign turns.
// Empty SessionID is the default Session — do not omitempty it away.
type RenderIntent struct {
	Kind      string        `json:"kind"`
	Text      string        `json:"text,omitempty"`
	Level     string        `json:"level,omitempty"`
	Title     string        `json:"title,omitempty"`
	Pairs     []SummaryPair `json:"pairs,omitempty"`
	Detail    string        `json:"detail,omitempty"`
	SessionID string        `json:"sessionId"`
}

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
