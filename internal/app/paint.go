package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/render/mdansi"
	"github.com/tomori/my-go-lite-agent/serve"
)

// turnRenderer is the CLI Render Medium (CONTEXT.md). Display types come from
// pluginsdk (ADR-0030 P2: Medium may paint with the contract package).
type turnRenderer struct {
	thinkingShown bool
	gotContent    bool
	started       bool
	streamBuf     strings.Builder
	reasonBuf     strings.Builder
	streamedLive  bool
}

func (r *turnRenderer) begin() {
	r.thinkingShown = false
	r.gotContent = false
	r.started = true
	r.streamBuf.Reset()
	r.reasonBuf.Reset()
	r.streamedLive = false
}

func (r *turnRenderer) clearThinking() {
	r.thinkingShown = false
}

func (r *turnRenderer) onStatus(status string) {
	if status != "running" || !r.started {
		return
	}
	if !r.gotContent && !r.thinkingShown {
		r.renderMessageText("dim", "Thinking…")
		r.thinkingShown = true
	}
}

func (r *turnRenderer) onStream(delta, channel string) {
	if delta == "" {
		return
	}
	if channel == "reasoning" {
		r.reasonBuf.WriteString(delta)
		if !r.thinkingShown {
			r.clearThinking()
			r.renderMessageText("dim", "Thinking…")
			r.thinkingShown = true
		}
		return
	}
	r.streamBuf.WriteString(delta)
	if !r.streamedLive {
		r.clearThinking()
		r.streamedLive = true
	}
	text := r.streamBuf.String()
	n := len([]rune(text))
	const maxLive = 120
	live := text
	if n > maxLive {
		runes := []rune(text)
		live = "…" + string(runes[n-maxLive:])
	}
	fmt.Printf("\r\x1b[2K\x1b[2m%s  (%d chars)\x1b[0m", strings.ReplaceAll(live, "\n", " "), n)
	r.gotContent = true
}

func (r *turnRenderer) clearProgress() {
	if r.streamedLive {
		fmt.Print("\r\x1b[2K")
		r.streamedLive = false
	}
}

func (r *turnRenderer) onRender(ri pluginsdk.RenderIntent) {
	r.clearThinking()
	r.clearProgress()
	switch ri.Kind {
	case pluginsdk.RenderMarkdownText:
		if ri.Text == "" {
			return
		}
		r.gotContent = true
		fmt.Print(mdansi.Render(ri.Text))
	case pluginsdk.RenderMessageText:
		r.renderMessageText(ri.Level, ri.Text)
	case pluginsdk.RenderSummaryText:
		r.gotContent = true
		r.renderSummaryText(ri.Title, ri.Pairs, ri.Detail)
	default:
		if ri.Text != "" {
			fmt.Println(ri.Text)
		}
	}
}

func (r *turnRenderer) renderMessageText(level, text string) {
	switch level {
	case "error":
		fmt.Println("✖ " + text)
	case "warn":
		fmt.Println("⚠ " + text)
	case "dim":
		fmt.Println("\x1b[2m" + text + "\x1b[0m")
	default:
		fmt.Println("· " + text)
	}
}

func (r *turnRenderer) renderSummaryText(title string, pairs []pluginsdk.SummaryPair, detail string) {
	if title != "" {
		fmt.Printf("⏺ %s\n", title)
	}
	for _, p := range pairs {
		fmt.Printf("  %s: %s\n", p.Key, p.Value)
	}
	if detail != "" {
		fmt.Print(mdansi.Indent(detail, "  "))
	}
}

func (r *turnRenderer) end(assistant string) {
	r.clearThinking()
	r.clearProgress()
	if !r.gotContent && assistant != "" {
		fmt.Print(mdansi.Render(assistant))
		r.gotContent = true
	}
	if !r.gotContent {
		fmt.Println()
	}
	r.started = false
}

// wireRenderer attaches a turnRenderer to Host live signals for one call.
func wireRenderer(srv *serve.Server, r *turnRenderer) func() {
	unsub := srv.Subscribe(&serve.Subscriber{OnEvent: func(e serve.Event) {
		switch e.Topic {
		case "stream":
			var p struct {
				Delta   string `json:"delta"`
				Channel string `json:"channel"`
			}
			_ = json.Unmarshal(marshalAny(e.Data), &p)
			r.onStream(p.Delta, p.Channel)
		case "status":
			var p struct {
				Status string `json:"status"`
			}
			_ = json.Unmarshal(marshalAny(e.Data), &p)
			r.onStatus(p.Status)
		case "presentation":
			if ri, ok := e.Data.(pluginsdk.RenderIntent); ok {
				r.onRender(ri)
			} else {
				var ri pluginsdk.RenderIntent
				_ = json.Unmarshal(marshalAny(e.Data), &ri)
				if ri.Kind != "" {
					r.onRender(ri)
				}
			}
		}
	}})
	return unsub
}

func marshalAny(v any) json.RawMessage {
	switch t := v.(type) {
	case json.RawMessage:
		return t
	case nil:
		return json.RawMessage(`{}`)
	default:
		b, _ := json.Marshal(v)
		return b
	}
}

// cliToolApproval is the CLI Medium face for agent.confirm (policy.ask).
// Deferred #3 special case: generic tool name + args only.
func cliToolApproval(tool string, arguments json.RawMessage, workspace, sessionID string) bool {
	fmt.Print("\r\x1b[2K")
	fmt.Printf("⚠ allow tool %s?\n", tool)
	if len(arguments) > 0 {
		fmt.Printf("  arguments: %s\n", string(arguments))
	}
	if workspace != "" {
		fmt.Printf("  workspace: %s\n", workspace)
	}
	fmt.Print("allow? [y/N] ")
	var line string
	_, _ = fmt.Scanln(&line)
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}
