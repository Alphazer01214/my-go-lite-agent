package serve

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Render kinds (protocol v2). See CONTEXT.md Presentation.
const (
	KindMarkdownText = "markdown_text"
	KindMessageText  = "message_text"
	KindSummaryText  = "summary_text"
)

// Truncation limits for summary_text display (rune-counted).
const (
	MaxSummaryValueLen = 120
	MaxSummaryPairs    = 8
	MaxSummaryDetail   = 400
)

// SummaryPair is one ordered key/value row on a summary_text card.
type SummaryPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RenderIntent is one classified main-window block for the Render Medium.
// JSON field names match pluginsdk so Web SSE and Frame payloads share one wire shape.
type RenderIntent struct {
	Kind   string        `json:"kind"`
	Text   string        `json:"text,omitempty"`
	Level  string        `json:"level,omitempty"`
	Title  string        `json:"title,omitempty"`
	Pairs  []SummaryPair `json:"pairs,omitempty"`
	Detail string        `json:"detail,omitempty"`
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

// JSONToPairs walks a JSON object in document order and returns up to max pairs.
// Remaining keys are returned as overflow text (to be merged into detail).
// Non-object JSON becomes a single "value" pair.
func JSONToPairs(raw json.RawMessage, max int) (pairs []SummaryPair, overflow string) {
	if len(raw) == 0 || max <= 0 {
		return nil, ""
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, ""
	}
	if trimmed[0] != '{' {
		return []SummaryPair{{Key: "value", Value: TruncateRunes(trimmed, MaxSummaryValueLen)}}, ""
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return []SummaryPair{{Key: "value", Value: TruncateRunes(trimmed, MaxSummaryValueLen)}}, ""
	}
	var rest []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := keyTok.(string)
		if !ok {
			break
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			break
		}
		if len(pairs) < max {
			pairs = append(pairs, SummaryPair{
				Key:   key,
				Value: TruncateRunes(compactJSON(val), MaxSummaryValueLen),
			})
			continue
		}
		rest = append(rest, key+": "+TruncateRunes(compactJSON(val), MaxSummaryValueLen))
	}
	return pairs, strings.Join(rest, "; ")
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// formatRunningLine builds the message_text shown when a tool starts.
func formatRunningLine(toolName string) string {
	return fmt.Sprintf("Running %s…", toolName)
}
