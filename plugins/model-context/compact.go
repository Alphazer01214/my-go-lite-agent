package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) compact(ctx context.Context, contextID string, in compactIn) (compactOut, error) {
	if in.SessionID == "" {
		return compactOut{}, pluginsdk.ErrCode("bad_arguments", "session_id is required")
	}
	keepLast := 20
	if in.KeepLast != nil && *in.KeepLast >= 0 {
		keepLast = *in.KeepLast
	}
	maxChars := 12000
	if in.MaxChars != nil && *in.MaxChars > 0 {
		maxChars = *in.MaxChars
	}
	payload, _ := json.Marshal(queryIn{SessionID: in.SessionID, Types: []string{"message"}})
	raw, err := p.call(ctx, contextID, "session", "query", payload)
	if err != nil {
		return compactOut{}, err
	}
	var queried queryOut
	if err := json.Unmarshal(raw, &queried); err != nil {
		return compactOut{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "decode session.query: "+err.Error())
	}
	if len(queried.Facts) <= keepLast || keepLast == len(queried.Facts) {
		return compactOut{SessionID: in.SessionID, Compacted: false}, nil
	}
	old := queried.Facts[:len(queried.Facts)-keepLast]
	summary := buildSummary(old, maxChars)
	meta, _ := json.Marshal(map[string]any{
		"active":             true,
		"covers_through_seq": old[len(old)-1].Seq,
		"generated_by":       "model-context.compact",
	})
	appendPayload, _ := json.Marshal(appendIn{
		SessionID: in.SessionID,
		Facts: []factIn{
			{Type: "context_summary", Role: "host", Content: summary, Meta: meta},
			{Type: "context_compaction", Role: "host", Meta: mustJSON(map[string]any{
				"source_from_seq": old[0].Seq,
				"source_to_seq":   old[len(old)-1].Seq,
				"messages":        len(old),
				"strategy":        "deterministic_digest",
			})},
		},
	})
	appendRaw, err := p.call(ctx, contextID, "session", "append", appendPayload)
	if err != nil {
		return compactOut{}, err
	}
	var appended appendOut
	_ = json.Unmarshal(appendRaw, &appended)
	out := compactOut{
		SessionID: in.SessionID, Compacted: true,
		SourceFrom: old[0].Seq, SourceTo: old[len(old)-1].Seq,
		Messages: len(old),
	}
	if len(appended.Seqs) > 0 {
		out.SummarySeq = appended.Seqs[0]
	}
	return out, nil
}

func buildSummary(facts []fact, maxChars int) string {
	var builder strings.Builder
	builder.WriteString("Earlier conversation summary:\n")
	for _, fact := range facts {
		content := strings.TrimSpace(fact.Content)
		if content == "" {
			continue
		}
		content = strings.Join(strings.Fields(content), " ")
		if len(content) > 400 {
			content = content[:400] + "..."
		}
		builder.WriteString("- ")
		builder.WriteString(fact.Role)
		builder.WriteString(": ")
		builder.WriteString(content)
		builder.WriteByte('\n')
		if builder.Len() >= maxChars {
			break
		}
	}
	if builder.Len() > maxChars {
		value := builder.String()[:maxChars]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
		return value
	}
	return builder.String()
}

func mustJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return body
}
