package app

import (
	"flag"
	"fmt"

	"github.com/tomori/my-go-lite-agent/serve"
)

// Server runs the liteagent-server entry: the Web Render Medium over the
// shared kernel, optionally combined with the CLI REPL (ADR-0011). All
// CLI-Medium-only surfaces live in liteagent-cli.
func Server() {
	progName = "liteagent-server"
	pluginsDir := flag.String("plugins", "", "plugin directory for assembly")
	assemblyPath := flag.String("assembly", "", "assembly config path (JSON plugins list)")
	dump := flag.Bool("dump", false, "dump assembly tree after resolve")
	repl := flag.Bool("repl", false, "interactive multi-turn REPL alongside the Web Medium")
	serveAddr := flag.String("serve", "", "start Web Medium on this address (e.g. 127.0.0.1:7788); combine with -repl")
	layoutPath := flag.String("layout", "", "layout.json path (default: layout.json beside cwd); required (ADR-0012)")
	debug := flag.Bool("debug", false, "print host Frame debug log to stderr")
	flag.Parse()
	if *debug {
		serve.SetDebug(true)
	}

	if *assemblyPath == "" {
		fatal(fmt.Errorf("-assembly is required"))
	}
	if *pluginsDir == "" {
		fatal(fmt.Errorf("-assembly requires -plugins"))
	}
	if *serveAddr == "" {
		fatal(fmt.Errorf("-serve is required — liteagent-server is the Web Medium entry; the CLI Medium lives in liteagent-cli"))
	}
	if err := runWebAndOptionalREPL(*pluginsDir, *assemblyPath, *serveAddr, *layoutPath, *repl, dump); err != nil {
		fatal(err)
	}
}
