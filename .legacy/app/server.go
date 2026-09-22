package app

import (
	"flag"
	"fmt"

	"github.com/tomori/my-go-lite-agent/serve"
)

// Server runs the liteagent-server entry: the Web Render Medium over the
// shared kernel.
func Server() {
	progName = "liteagent-server"
	pluginsDir := flag.String("plugins", "", "plugin directory for Discovery")
	assemblyPath := flag.String("assembly", "", "deprecated (ADR-0021): ignored whitelist; Autostart+dependsOn is used")
	dump := flag.Bool("dump", false, "dump mount plan after resolve")
	serveAddr := flag.String("serve", "", "start Web Medium on this address (e.g. 127.0.0.1:7788) (required)")
	layoutPath := flag.String("layout", "", "layout.json path (default: layout.json beside cwd); required (ADR-0012)")
	debug := flag.Bool("debug", false, "print host Frame debug log to stderr")
	//logDir := flag.String("log", "", "log directory (default: cwd)")
	flag.Parse()
	if *debug {
		serve.SetDebug(true)
	}

	if *pluginsDir == "" {
		fatal(fmt.Errorf("-plugins is required"))
	}
	if *serveAddr == "" {
		fatal(fmt.Errorf("-serve is required — liteagent-server is the Web Medium entry"))
	}
	if err := runWeb(*pluginsDir, *assemblyPath, *serveAddr, *layoutPath, dump); err != nil {
		fatal(err)
	}
}
