package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tomori/my-go-lite-agent/render/mdansi"
	"github.com/tomori/my-go-lite-agent/serve"
)

// sessionAgentOpts carries Host CLI options for the session/agent/loop path.
type sessionAgentOpts struct {
	pluginsDir    *string
	assemblyPath  *string
	appendJSON    *string
	derive        *bool
	query         *bool
	requestJSON   *string
	injectJSON    *string
	turnInput     *string
	dump          *bool
	invokePlugin  *string
	callCap       *string
	invokePayload *string
	frameCap      *string
	frameMethod   *string
	contextList   *int
	cards         *bool
	workspace     string
}

// turnRenderer is the CLI Render Medium (CONTEXT.md).
// Content is classified as markdown_text | message_text | summary_text.
// Stream deltas carry channel (content|reasoning) so thinking never mixes
// into the live answer preview (fixes repeated thinking print).
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
	// Thinking is a full message_text line; nothing to erase on the same row.
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
	// Reasoning is ephemeral presentation only — accumulate separately and
	// show a short dim preview; never append into the answer streamBuf.
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
	// Dim live tokens; durable body is still painted as markdown_text at settle.
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
	// Single-line live preview; settle erases this row.
	fmt.Printf("\r\x1b[2K\x1b[2m%s  (%d chars)\x1b[0m", strings.ReplaceAll(live, "\n", " "), n)
	r.gotContent = true
}

func (r *turnRenderer) onTool(name string, args json.RawMessage) {
	// Tool display is owned by message_text (start) + summary_text (end).
	r.clearThinking()
	// New tool hop: drop previous live preview so thinking/content don't pile up.
	r.streamBuf.Reset()
	r.streamedLive = false
}

func (r *turnRenderer) clearProgress() {
	if r.streamedLive {
		fmt.Print("\r\x1b[2K")
		r.streamedLive = false
	}
}

func (r *turnRenderer) onRender(ri serve.RenderIntent) {
	r.clearThinking()
	r.clearProgress()
	switch string(ri.Kind) {
	case serve.KindMarkdownText:
		if ri.Text == "" {
			return
		}
		r.gotContent = true
		fmt.Print(mdansi.Render(ri.Text))
	case serve.KindMessageText:
		r.renderMessageText(ri.Level, ri.Text)
	case serve.KindSummaryText:
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

func (r *turnRenderer) renderSummaryText(title string, pairs []serve.SummaryPair, detail string) {
	if title != "" {
		fmt.Printf("⏺ %s\n", title)
	}
	for _, p := range pairs {
		fmt.Printf("  %s: %s\n", p.Key, p.Value)
	}
	if detail != "" {
		fmt.Print(mdansi.Indent(serve.TruncateRunes(detail, serve.MaxSummaryDetail), "  "))
	}
}

func (r *turnRenderer) end(assistant string) {
	r.clearThinking()
	r.clearProgress()
	// Fallback when Host did not emit settle markdown_text.
	if !r.gotContent && assistant != "" {
		fmt.Print(mdansi.Render(assistant))
		r.gotContent = true
	}
	if !r.gotContent {
		fmt.Println()
	}
	r.started = false
}

// wireRenderer attaches a turnRenderer to Host live signals for one RunTurn.
// Live signals are broadcast events (ADR-0029): subscribing keeps Multi
// Medium coexistence intact — the CLI face reads the same stream/presentation
// events the Web medium consumes, without overwriting any hook.
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
			if ri, ok := e.Data.(serve.RenderIntent); ok {
				r.onRender(ri)
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
// Host/CLI stay plugin-agnostic: only the generic tool name + args are shown.
func cliToolApproval(tool string, arguments json.RawMessage, workspace, sessionID string) bool {
	// Clear any live stream row so the prompt is not painted over.
	fmt.Print("\r\x1b[2K")
	fmt.Printf("⚠ allow tool %s?\n", tool)
	if len(arguments) > 0 {
		fmt.Printf("  arguments: %s\n", string(arguments))
	}
	if workspace != "" {
		fmt.Printf("  workspace: %s\n", workspace)
	}
	fmt.Print("  [y/N] ")
	_ = os.Stdout.Sync()
	var line string
	_, _ = fmt.Scanln(&line)
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// runSessionAgent mounts Plugins then runs session ops, optional Agent Loop turn, and optional invoke.
func runSessionAgent(opts sessionAgentOpts) error {
	plan, _, err := resolveAssembly(*opts.pluginsDir, *opts.assemblyPath, opts.dump != nil && *opts.dump)
	if err != nil {
		return err
	}
	srv, err := startMounted(*opts.pluginsDir, plan)
	if err != nil {
		return err
	}
	defer func() {
		if opts.cards != nil && *opts.cards {
			printCards(srv.Cards())
		}
		_ = srv.Close()
	}()

	// Bind Workspace only when a Turn will run (ADR-0020). Session-only
	// diagnostics must not insert session_meta facts that shift seq coverage.
	if opts.workspace != "" && opts.turnInput != nil && *opts.turnInput != "" {
		// Soft: session plugin may be absent in bare assemblies.
		_, _ = srv.CallByCap(serve.SessionCap, "create", serve.MarshalPayload(map[string]any{
			"sessionId": "default", "workspace": opts.workspace,
		}))
	}
	serve.RegisterApproval(srv, cliToolApproval)

	if *opts.appendJSON != "" {
		var facts []map[string]any
		if err := json.Unmarshal([]byte(*opts.appendJSON), &facts); err != nil {
			return fmt.Errorf("parse -session-append: %w", err)
		}
		last := 0
		for _, fact := range facts {
			out, err := srv.CallByCap(serve.SessionCap, "append", serve.MarshalPayload(fact))
			if err != nil {
				return err
			}
			var res struct {
				Seq int `json:"seq"`
			}
			if len(out) > 0 {
				_ = json.Unmarshal(out, &res)
			}
			last = res.Seq
		}
		fmt.Printf("append ok count=%d lastSeq=%d\n", len(facts), last)
	}

	if opts.injectJSON != nil && *opts.injectJSON != "" {
		var msgs []serve.Message
		if err := json.Unmarshal([]byte(*opts.injectJSON), &msgs); err != nil {
			return fmt.Errorf("parse -agent-inject: %w", err)
		}
		out, err := srv.AgentInject(serve.MarshalPayload(map[string]any{"messages": msgs}))
		if err != nil {
			return err
		}
		fmt.Printf("inject ok count=%d lastSeq=%d\n", out["count"], out["lastSeq"])
	}

	// Invoke before turn so fixtures can register Prompt Segments or warm state.
	if *opts.invokePlugin != "" {
		payload := map[string]string{}
		if *opts.callCap != "" {
			payload["cap"] = *opts.callCap
		}
		var rawPayload json.RawMessage
		if opts.invokePayload != nil && *opts.invokePayload != "" {
			rawPayload = json.RawMessage(*opts.invokePayload)
		} else {
			rawPayload = serve.MarshalPayload(payload)
		}
		capName, methodName := "demo", "invoke"
		if opts.frameCap != nil && *opts.frameCap != "" {
			capName = *opts.frameCap
		}
		if opts.frameMethod != nil && *opts.frameMethod != "" {
			methodName = *opts.frameMethod
		}
		out, err := srv.CallByCap(capName, methodName, rawPayload)
		if err != nil {
			return fmt.Errorf("invoke %s: %w", *opts.invokePlugin, err)
		}
		fmt.Printf("invoke ok payload=%s\n", string(out))
	}

	if *opts.turnInput != "" {
		r := &turnRenderer{}
		restore := wireRenderer(srv, r)
		r.begin()
		out, err := srv.RunTurn(*opts.turnInput)
		if err != nil {
			r.end("")
			restore()
			return err
		}
		r.end(out.Assistant)
		restore()
		fmt.Printf("turn ok user=%s assistant=%q chunks=%d tools=%v\n",
			out.User, out.Assistant, len(out.Chunks), out.ToolCalls)
		for i, c := range out.Chunks {
			fmt.Printf("chunk[%d]=%q\n", i, c)
		}
		for i, name := range out.ToolCalls {
			fmt.Printf("tool_call[%d]=%s\n", i, name)
		}
		if u, err := srv.CallByCap("context", "usage", json.RawMessage(`{}`)); err == nil && len(u) > 0 {
			body, _ := json.Marshal(json.RawMessage(u))
			fmt.Printf("context usage=%s\n", body)
		}
	}

	if *opts.derive {
		out, err := srv.CallByCap(serve.SessionCap, "derive", json.RawMessage(`{}`))
		if err != nil {
			return err
		}
		var res struct {
			Messages []serve.Message `json:"messages"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Messages == nil {
			res.Messages = []serve.Message{}
		}
		body, _ := json.Marshal(res.Messages)
		fmt.Printf("derive ok messages=%s\n", body)
	}

	if *opts.query {
		out, err := srv.CallByCap(serve.SessionCap, "query", serve.MarshalPayload(map[string]any{"afterSeq": 0, "limit": 0}))
		if err != nil {
			return err
		}
		var res struct {
			Facts []map[string]any `json:"facts"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Facts == nil {
			res.Facts = []map[string]any{}
		}
		body, _ := json.Marshal(res.Facts)
		fmt.Printf("query ok facts=%s\n", body)
	}

	if opts.contextList != nil && *opts.contextList > 0 {
		out, err := srv.CallByCap("context", "listContext", serve.MarshalPayload(map[string]any{"n": *opts.contextList}))
		if err != nil {
			return err
		}
		var res struct {
			Messages []serve.Message `json:"messages"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Messages == nil {
			res.Messages = []serve.Message{}
		}
		body, _ := json.Marshal(res.Messages)
		fmt.Printf("context list count=%d messages=%s\n", len(res.Messages), body)
	}

	if *opts.requestJSON != "" {
		var claimed []serve.Message
		if err := json.Unmarshal([]byte(*opts.requestJSON), &claimed); err != nil {
			return fmt.Errorf("parse -agent-request: %w", err)
		}
		out, err := srv.AgentRequest("", claimed)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(out)
		fmt.Printf("agent/request ok rebuilt=%v messages=%s\n", out.Rebuilt, body)
	}
	return nil
}
