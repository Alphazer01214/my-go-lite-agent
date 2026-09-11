// Command host is the thin kernel entry.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
	"github.com/tomori/my-go-lite-agent/serve"
)

func main() {
	pluginPath := flag.String("plugin", "", "path to plugin executable (single Frame round-trip)")
	discoverDir := flag.String("discover", "", "scan plugin directory and list discovered plugins (no mount)")
	pluginsDir := flag.String("plugins", "", "plugin directory for assembly")
	assemblyPath := flag.String("assembly", "", "assembly config path (JSON plugins list)")
	dump := flag.Bool("dump", false, "dump assembly tree after resolve")
	invokePlugin := flag.String("invoke", "", "after mount, Host-initiated req to this plugin (serve mode)")
	callCap := flag.String("call-cap", "", "Capability name for the consumer to call via Host (default echo)")
	callPlugin := flag.String("call-plugin", "", "after mount, Host-initiated req directly to this plugin name")
	sessionAppend := flag.String("session-append", "", "JSON array of facts to append via session")
	sessionDerive := flag.Bool("session-derive", false, "print session.derive Model Context")
	sessionQuery := flag.Bool("session-query", false, "print session.query facts")
	agentRequest := flag.String("agent-request", "", "JSON array of claimed model messages for agent/request invariant check")
	agentInject := flag.String("agent-inject", "", "JSON array of messages to append via agent.inject (does not start a turn)")
	turnInput := flag.String("turn", "", "run one default-Loop turn with this user input (requires session + llm)")
	repl := flag.Bool("repl", false, "interactive multi-turn REPL (same process/session; Ctrl+C or exit to quit)")
	verbose := flag.Bool("verbose", false, "show tool calls and stream details (REPL/-turn); default prints Thinking… until body text")
	invokePayload := flag.String("invoke-payload", "", "JSON payload for -invoke (overrides -call-cap)")
	audit := flag.Bool("audit", false, "print built-in Waterfall audit entries after the run")
	cards := flag.Bool("cards", false, "print Presentation Cards observed during the run")
	flag.Parse()

	switch {
	case *discoverDir != "":
		if err := runDiscover(*discoverDir); err != nil {
			fatal(err)
		}
	case *assemblyPath != "":
		if *pluginsDir == "" {
			fatal(fmt.Errorf("-assembly requires -plugins"))
		}
		if *repl {
			if err := runREPL(*pluginsDir, *assemblyPath, dump, *verbose); err != nil {
				fatal(err)
			}
			return
		}
		if *sessionAppend != "" || *sessionDerive || *sessionQuery || *agentRequest != "" || *agentInject != "" || *turnInput != "" || *cards {
			opts := sessionAgentOpts{
				pluginsDir:    pluginsDir,
				assemblyPath:  assemblyPath,
				appendJSON:    sessionAppend,
				derive:        sessionDerive,
				query:         sessionQuery,
				requestJSON:   agentRequest,
				injectJSON:    agentInject,
				turnInput:     turnInput,
				dump:          dump,
				invokePlugin:  invokePlugin,
				callCap:       callCap,
				invokePayload: invokePayload,
				audit:         audit,
				cards:         cards,
				verbose:       verbose,
			}
			if err := runSessionAgent(opts); err != nil {
				fatal(err)
			}
			return
		}
		if *invokePlugin != "" {
			if err := runServe(*pluginsDir, *assemblyPath, *invokePlugin, *callCap, *dump, *audit); err != nil {
				fatal(err)
			}
			return
		}
		if *callPlugin != "" {
			if err := runCallPlugin(*pluginsDir, *assemblyPath, *callPlugin, *dump); err != nil {
				fatal(err)
			}
			return
		}
		if err := runAssembly(*pluginsDir, *assemblyPath, *dump); err != nil {
			fatal(err)
		}
	case *pluginPath != "":
		if err := runEchoRoundtrip(*pluginPath); err != nil {
			fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "host: -plugin, -discover, or -assembly is required")
		os.Exit(2)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "host: %v\n", err)
	os.Exit(1)
}

func printAudit(entries []serve.AuditEntry) {
	for _, e := range entries {
		fmt.Printf("audit from=%s cap=%s method=%s action=%s", e.From, e.Cap, e.Method, e.Action)
		if e.Reason != "" {
			fmt.Printf(" reason=%s", e.Reason)
		}
		fmt.Println()
	}
}

func printCards(cards []serve.PresentationCard) {
	for i, c := range cards {
		fmt.Printf("card[%d] type=%s tool=%s data=%s\n", i, c.CardType, c.Tool, string(c.Data))
	}
}

func runDiscover(dir string) error {
	res := discovery.Scan(dir)
	printDiscovery(res)
	if len(res.Errors) > 0 {
		return fmt.Errorf("discovery failed for %d director%s", len(res.Errors), plural(len(res.Errors)))
	}
	return nil
}

func printDiscovery(res discovery.Result) {
	for _, p := range res.Plugins {
		m := p.Manifest
		fmt.Printf("%s\t%s\tprotocol=%d\tprovides=[%s]\tconsumes=[%s]\tentry=%s\n",
			m.Name, m.Version, m.Protocol,
			strings.Join(m.Provides, ","),
			strings.Join(m.Consumes, ","),
			m.Entry,
		)
	}
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "discover error: %v\n", e)
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func runAssembly(pluginsDir, assemblyPath string, dump bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}

	for _, p := range plan.Mounted {
		if err := probePlugin(p); err != nil {
			return fmt.Errorf("mount %s: %w", p.Manifest.Name, err)
		}
	}

	if dump {
		dumpAssembly(plan, res)
	}
	return nil
}

func dumpAssembly(plan assembly.Plan, res discovery.Result) {
	fmt.Println("assembly:")
	for _, p := range plan.Mounted {
		m := p.Manifest
		fmt.Printf("  mounted name=%s dir=%s entry=%s provides=[%s]\n",
			m.Name, p.Dir, m.Entry, strings.Join(m.Provides, ","))
	}
	for _, p := range plan.Unmounted {
		m := p.Manifest
		fmt.Printf("  available name=%s mounted=false\n", m.Name)
	}
	_ = res
}

// probePlugin starts the Plugin, completes one echo Frame, then shuts it down.
func probePlugin(p discovery.Found) error {
	fmt.Printf("mount name=%s\n", p.Manifest.Name)
	return roundtrip(p.Manifest.ResolveEntry(p.Dir), p.Manifest.Name)
}

func runEchoRoundtrip(pluginPath string) error {
	return roundtrip(pluginPath, "1")
}

func roundtrip(pluginPath, id string) error {
	cmd := exec.Command(pluginPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start plugin: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	req := &protocol.Frame{
		V:       1,
		ID:      id,
		Type:    protocol.TypeReq,
		Cap:     "echo",
		Method:  "echo",
		Payload: json.RawMessage(`{"hello":"world"}`),
	}
	if err := protocol.WriteFrame(stdin, req); err != nil {
		return fmt.Errorf("write req: %w", err)
	}
	res, err := protocol.ReadFrame(stdout)
	if err != nil {
		return fmt.Errorf("read res: %w", err)
	}
	if res.Type != protocol.TypeRes || res.ID != req.ID {
		return fmt.Errorf("unexpected frame: type=%s id=%s", res.Type, res.ID)
	}
	if res.Error != nil {
		return fmt.Errorf("plugin error: %w", res.Error)
	}
	fmt.Printf("ok id=%s payload=%s\n", res.ID, string(res.Payload))
	return nil
}

func runServe(pluginsDir, assemblyPath, invokePlugin, callCap string, dump, audit bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}
	if dump {
		dumpAssembly(plan, res)
	}

	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() {
		if audit {
			printAudit(srv.Audit())
		}
		_ = srv.Close()
	}()

	payload := map[string]string{}
	if callCap != "" {
		payload["cap"] = callCap
	}
	frame := &protocol.Frame{
		V:       1,
		Type:    protocol.TypeReq,
		Cap:     "demo",
		Method:  "invoke",
		Payload: serve.MarshalPayload(payload),
	}
	out, err := srv.Call(invokePlugin, frame)
	if err != nil {
		return fmt.Errorf("invoke %s: %w", invokePlugin, err)
	}
	if out.Error != nil {
		fmt.Printf("invoke error code=%s msg=%s\n", out.Error.Code, out.Error.Message)
		return fmt.Errorf("invoke failed: %s", out.Error.Code)
	}
	fmt.Printf("invoke ok payload=%s\n", string(out.Payload))
	return nil
}

func runCallPlugin(pluginsDir, assemblyPath, name string, dump bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}
	if dump {
		dumpAssembly(plan, res)
	}
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	frame := &protocol.Frame{
		V:       1,
		Type:    protocol.TypeReq,
		Cap:     name,
		Method:  "echo",
		Payload: json.RawMessage(`{"hello":"lifecycle"}`),
	}
	out, err := srv.Call(name, frame)
	if err != nil {
		return err
	}
	if out.Error != nil {
		return fmt.Errorf("call failed: %s: %s", out.Error.Code, out.Error.Message)
	}
	fmt.Printf("ok plugin=%s payload=%s\n", name, string(out.Payload))
	return nil
}

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
	verbose       *bool
}

// turnRenderer is the CLI Render Medium for one Turn (CONTEXT.md Render Medium).
type turnRenderer struct {
	verbose       bool
	thinkingShown bool
	gotContent    bool
	started       bool
}

func (r *turnRenderer) begin() {
	r.thinkingShown = false
	r.gotContent = false
	r.started = true
}

func (r *turnRenderer) onStatus(status string) {
	if status != "running" || !r.started {
		return
	}
	// Compact mode: no body yet → Claude-Code style placeholder.
	if !r.verbose && !r.gotContent && !r.thinkingShown {
		fmt.Print("Thinking…")
		r.thinkingShown = true
	}
}

func (r *turnRenderer) onStream(delta string) {
	if delta == "" {
		return
	}
	if !r.gotContent {
		if r.thinkingShown {
			fmt.Println()
			r.thinkingShown = false
		}
		r.gotContent = true
	}
	if r.verbose {
		// verbose still prints body; tools already annotated.
	}
	fmt.Print(delta)
}

func (r *turnRenderer) onTool(name string, args json.RawMessage) {
	if r.thinkingShown {
		fmt.Println()
		r.thinkingShown = false
	}
	if !r.verbose {
		// Compact: hide tool noise; keep Thinking… until body text (Claude-Code style).
		if !r.gotContent {
			fmt.Print("Thinking…")
			r.thinkingShown = true
		}
		return
	}
	fmt.Printf("⏺ %s(%s)\n", name, compactArgs(args))
}

func (r *turnRenderer) end() {
	if r.thinkingShown {
		fmt.Println()
		r.thinkingShown = false
	}
	if !r.gotContent {
		fmt.Println()
	}
	r.started = false
}

func compactArgs(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	s := string(args)
	if len(s) > 120 {
		s = s[:117] + "…"
	}
	return s
}

// wireRenderer attaches a turnRenderer to Host live hooks for one RunTurn.
func wireRenderer(srv *serve.Server, r *turnRenderer) (restore func()) {
	prevDelta, prevStatus, prevTool := srv.OnStreamDelta, srv.OnStatus, srv.OnToolCall
	srv.OnStreamDelta = r.onStream
	srv.OnStatus = r.onStatus
	srv.OnToolCall = r.onTool
	return func() {
		srv.OnStreamDelta, srv.OnStatus, srv.OnToolCall = prevDelta, prevStatus, prevTool
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
			V:       1,
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
		r := &turnRenderer{verbose: opts.verbose != nil && *opts.verbose}
		restore := wireRenderer(srv, r)
		r.begin()
		out, err := srv.RunTurn(*opts.turnInput)
		r.end()
		restore()
		if err != nil {
			return err
		}
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

// runREPL mounts Plugins and runs an interactive multi-turn loop on one Session.
func runREPL(pluginsDir, assemblyPath string, dump *bool, verbose bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}
	if dump != nil && *dump {
		dumpAssembly(plan, res)
	}
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	// Clean shutdown on Ctrl+C so plugin processes are not orphaned.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nshutting down…")
		_ = srv.Close()
		os.Exit(0)
	}()

	srv.OnStreamDelta = nil
	srv.OnStatus = nil
	srv.OnToolCall = nil

	if verbose {
		fmt.Println("lite agent REPL (verbose) — type a message; exit/quit or Ctrl+C to leave.")
	} else {
		fmt.Println("lite agent REPL — type a message; exit/quit or Ctrl+C to leave.")
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Print("> ")
		if !in.Scan() {
			fmt.Println()
			break
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if low == "exit" || low == "quit" {
			break
		}
		r := &turnRenderer{verbose: verbose}
		restore := wireRenderer(srv, r)
		r.begin()
		out, err := srv.RunTurn(line)
		r.end()
		restore()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		// If the model never streamed body text, print the durable assistant reply.
		if out.Assistant != "" && !r.gotContent {
			fmt.Println(out.Assistant)
		}
	}
	return in.Err()
}
