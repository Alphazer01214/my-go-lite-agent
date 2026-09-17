package app

import (
	"flag"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/serve"
)

// orDefault returns def when v is empty (flag defaults per entry).
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// CLI runs the liteagent-cli entry (ADR-0030 L0-only startup):
// -plugins / -repl / -debug / -discover / -dump plus generic diagnostics
// (-invoke / -frame-* / -plugin / -call-plugin). Domain actions go through
// mounted plugin commands / REPL, not process argv.
func CLI() {
	progName = "liteagent-cli"
	enableVirtualTerminal()
	pluginPath := flag.String("plugin", "", "path to plugin executable (single Frame round-trip)")
	discoverDir := flag.String("discover", "", "scan plugin directory and list discovered plugins (no mount)")
	pluginsDir := flag.String("plugins", "", "plugin directory for Discovery")
	assemblyPath := flag.String("assembly", "", "deprecated (ADR-0021): ignored whitelist; Autostart+dependsOn is used")
	dump := flag.Bool("dump", false, "dump assembly tree after resolve")
	invokePlugin := flag.String("invoke", "", "after mount, point-named req to this plugin (diagnostic)")
	callCap := flag.String("call-cap", "", "payload cap field for fixture consumers")
	callPlugin := flag.String("call-plugin", "", "after mount, Host-initiated req directly to this plugin name")
	repl := flag.Bool("repl", false, "interactive multi-turn REPL (same process/session; Ctrl+C or /exit to quit)")
	invokePayload := flag.String("invoke-payload", "", "JSON payload for -invoke")
	frameCap := flag.String("frame-cap", "", "Frame Capability for -invoke / -plugin (plugin dispatch key)")
	frameMethod := flag.String("frame-method", "", "Frame method for -invoke / -plugin")
	debug := flag.Bool("debug", false, "print host Frame debug log to stderr")
	// Hidden test-only domain flags (ADR-0030): registered only under L0_TEST_COMPAT=1
	// so product argv/help stay L0-only while integration tests keep working.
	var (
		legacySessionAppend  string
		legacySessionDerive  bool
		legacySessionQuery   bool
		legacyAgentRequest   string
		legacyAgentInject    string
		legacyTurn           string
		legacyContextList    int
		legacyCards          bool
		legacyWorkspace      string
	)
	if os.Getenv("L0_TEST_COMPAT") == "1" {
		flag.StringVar(&legacySessionAppend, "session-append", "", "test-compat: JSON facts to append")
		flag.BoolVar(&legacySessionDerive, "session-derive", false, "test-compat: print derive")
		flag.BoolVar(&legacySessionQuery, "session-query", false, "test-compat: print query")
		flag.StringVar(&legacyAgentRequest, "agent-request", "", "test-compat: claimed messages")
		flag.StringVar(&legacyAgentInject, "agent-inject", "", "test-compat: inject messages")
		flag.StringVar(&legacyTurn, "turn", "", "test-compat: one loop.turn")
		flag.IntVar(&legacyContextList, "context-list", 0, "test-compat: listContext n")
		flag.BoolVar(&legacyCards, "cards", false, "test-compat: print cards")
		flag.StringVar(&legacyWorkspace, "workspace", "", "test-compat: workspace root")
	}
	flag.Parse()
	if *debug {
		serve.SetDebug(true)
	}

	// Test-compat: domain flags + optional -invoke run on one Server (order matters).
	if os.Getenv("L0_TEST_COMPAT") == "1" {
		hasLegacy := legacySessionAppend != "" || legacySessionDerive || legacySessionQuery ||
			legacyAgentRequest != "" || legacyAgentInject != "" || legacyTurn != "" || legacyContextList > 0 || legacyCards || legacyWorkspace != ""
		if hasLegacy || *invokePlugin != "" {
			if *pluginsDir == "" {
				fatal(fmt.Errorf("-plugins is required"))
			}
			if err := runLegacyDomain(*pluginsDir, *assemblyPath, *dump, &legacyDomainFlags{
				sessionAppend: legacySessionAppend,
				sessionDerive: legacySessionDerive,
				sessionQuery:  legacySessionQuery,
				agentRequest:  legacyAgentRequest,
				agentInject:   legacyAgentInject,
				turnInput:     legacyTurn,
				contextList:   legacyContextList,
				cards:         legacyCards,
				workspace:     legacyWorkspace,
				invokePlugin:  *invokePlugin,
				invokePayload: *invokePayload,
				frameCap:      *frameCap,
				frameMethod:   *frameMethod,
				callCap:       *callCap,
			}); err != nil {
				fatal(err)
			}
			return
		}
	}

	switch {
	case *discoverDir != "":
		if err := runDiscover(*discoverDir); err != nil {
			fatal(err)
		}
	case *pluginsDir != "" && (*repl || *invokePlugin != "" || *callPlugin != "" || *dump):
		if *repl {
			if err := runREPL(*pluginsDir, *assemblyPath, dump, ""); err != nil {
				fatal(err)
			}
			return
		}
		if *invokePlugin != "" {
			opts := invokeOpts{
				pluginsDir:    pluginsDir,
				assemblyPath:  assemblyPath,
				dump:          dump,
				invokePlugin:  invokePlugin,
				callCap:       callCap,
				invokePayload: invokePayload,
				frameCap:      frameCap,
				frameMethod:   frameMethod,
			}
			if err := runInvoke(opts); err != nil {
				fatal(err)
			}
			return
		}
		if *callPlugin != "" {
			if err := runCallPlugin(*pluginsDir, *assemblyPath, *callPlugin, *dump, orDefault(*frameMethod, pluginsdk.ProbeMethod)); err != nil {
				fatal(err)
			}
			return
		}
		if err := runAssembly(*pluginsDir, *assemblyPath, *dump, orDefault(*frameCap, pluginsdk.ProbeCap), orDefault(*frameMethod, pluginsdk.ProbeMethod)); err != nil {
			fatal(err)
		}
	case *assemblyPath != "":
		if *pluginsDir == "" {
			fatal(fmt.Errorf("-assembly requires -plugins"))
		}
		if err := runAssembly(*pluginsDir, *assemblyPath, *dump, orDefault(*frameCap, pluginsdk.ProbeCap), orDefault(*frameMethod, pluginsdk.ProbeMethod)); err != nil {
			fatal(err)
		}
	case *pluginPath != "":
		if err := runPluginRoundtrip(*pluginPath, orDefault(*frameCap, pluginsdk.ProbeCap), orDefault(*frameMethod, pluginsdk.ProbeMethod)); err != nil {
			fatal(err)
		}
	default:
		fmt.Fprintln(os.Stderr, "liteagent-cli: -plugin, -discover, or -plugins is required")
		os.Exit(2)
	}
}
