package app

import (
	"flag"
	"fmt"
	"os"
)

// CLI runs the liteagent-cli entry: the CLI Render Medium over the shared
// kernel — REPL, one-shot turns, session ops, and plugin diagnostics
// (ADR-0011: the CLI Medium owns everything but the Web Medium).
func CLI() {
	progName = "liteagent-cli"
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
	repl := flag.Bool("repl", false, "interactive multi-turn REPL (same process/session; Ctrl+C or /exit to quit)")
	invokePayload := flag.String("invoke-payload", "", "JSON payload for -invoke (overrides -call-cap)")
	frameCap := flag.String("frame-cap", "", "Frame Capability for -invoke (default demo)")
	frameMethod := flag.String("frame-method", "", "Frame method for -invoke (default invoke)")
	contextList := flag.Int("context-list", 0, "print last N Model Context messages from Context Manager (0=off)")
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
			if err := runREPL(*pluginsDir, *assemblyPath, dump); err != nil {
				fatal(err)
			}
			return
		}
		if *sessionAppend != "" || *sessionDerive || *sessionQuery || *agentRequest != "" || *agentInject != "" || *turnInput != "" || *cards || *contextList > 0 || *invokePlugin != "" {
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
				frameCap:      frameCap,
				frameMethod:   frameMethod,
				contextList:   contextList,
				cards:         cards,
			}
			if err := runSessionAgent(opts); err != nil {
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
		fmt.Fprintln(os.Stderr, "liteagent-cli: -plugin, -discover, or -assembly is required")
		os.Exit(2)
	}
}
