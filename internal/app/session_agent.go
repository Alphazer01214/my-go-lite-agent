package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
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
	audit         *bool
	cards         *bool
}

// turnRenderer is the CLI Render Medium (CONTEXT.md).
// Content is classified as markdown_text | message_text | summary_text.
type turnRenderer struct {
	thinkingShown bool
	gotContent    bool
	started       bool
	streamBuf     strings.Builder
	streamedLive  bool
}

func (r *turnRenderer) begin() {
	r.thinkingShown = false
	r.gotContent = false
	r.started = true
	r.streamBuf.Reset()
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

func (r *turnRenderer) onStream(delta string) {
	if delta == "" {
		return
	}
	r.streamBuf.WriteString(delta)
	// Dim one-line progress; durable body is painted as markdown_text at settle.
	if !r.streamedLive {
		r.clearThinking()
		r.streamedLive = true
	}
	n := len([]rune(r.streamBuf.String()))
	fmt.Printf("\r\x1b[2K\x1b[2mGenerating… %d chars\x1b[0m", n)
	r.gotContent = true
}

func (r *turnRenderer) onTool(name string, args json.RawMessage) {
	// Tool display is owned by message_text (start) + summary_text (end).
	r.clearThinking()
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
	switch ri.Kind {
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

// wireRenderer attaches a turnRenderer to Host live hooks for one RunTurn.
func wireRenderer(srv *serve.Server, r *turnRenderer) (restore func()) {
	prevDelta, prevStatus, prevTool, prevRender := srv.OnStreamDelta, srv.OnStatus, srv.OnToolCall, srv.OnRender
	srv.OnStreamDelta = r.onStream
	srv.OnStatus = r.onStatus
	srv.OnToolCall = r.onTool
	srv.OnRender = r.onRender
	return func() {
		srv.OnStreamDelta, srv.OnStatus, srv.OnToolCall, srv.OnRender = prevDelta, prevStatus, prevTool, prevRender
	}
}

// runSessionAgent mounts Plugins then runs session ops, optional default Loop turn, and optional invoke.
func runSessionAgent(opts sessionAgentOpts) error {
	cfg, err := assembly.Load(*opts.assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(*opts.pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	printRejected(plan.Rejected)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}
	if *opts.dump {
		dumpAssembly(plan, res)
	}
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() {
		if opts.audit != nil && *opts.audit {
			printAudit(srv.Audit())
		}
		if opts.cards != nil && *opts.cards {
			printCards(srv.Cards())
		}
		_ = srv.Close()
	}()

	if *opts.appendJSON != "" {
		var facts []map[string]any
		if err := json.Unmarshal([]byte(*opts.appendJSON), &facts); err != nil {
			return fmt.Errorf("parse -session-append: %w", err)
		}
		seq, err := srv.AppendSessionFacts("", facts)
		if err != nil {
			return err
		}
		fmt.Printf("append ok count=%d lastSeq=%d\n", len(facts), seq)
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
		frame := &protocol.Frame{
			V:       protocol.Version,
			Type:    protocol.TypeReq,
			Cap:     "demo",
			Method:  "invoke",
			Payload: rawPayload,
		}
		out, err := srv.Call(*opts.invokePlugin, frame)
		if err != nil {
			return fmt.Errorf("invoke %s: %w", *opts.invokePlugin, err)
		}
		if out.Error != nil {
			fmt.Printf("invoke error code=%s msg=%s\n", out.Error.Code, out.Error.Message)
			return fmt.Errorf("invoke failed: %s", out.Error.Code)
		}
		fmt.Printf("invoke ok payload=%s\n", string(out.Payload))
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
	}

	if *opts.derive {
		msgs, err := srv.DeriveMessages("")
		if err != nil {
			return err
		}
		body, _ := json.Marshal(msgs)
		fmt.Printf("derive ok messages=%s\n", body)
	}

	if *opts.query {
		facts, err := srv.QuerySessionFacts("", 0, 0)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(facts)
		fmt.Printf("query ok facts=%s\n", body)
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
