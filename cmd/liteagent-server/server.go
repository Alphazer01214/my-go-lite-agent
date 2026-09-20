package main

import (
	"flag"
	"fmt"
	"os"
)

func Run() {
	pluginsDir := flag.String("plugins", "", "plugin directory for Discovery")
	addr := flag.String("addr", "", "start Web Medium on this address (e.g. 127.0.0.1:8080) (required)")
	layout := flag.String("layout", "", "path to layout.json (default: layout.json)")
	flag.Parse()
	if *pluginsDir == "" {
		fmt.Fprintf(os.Stderr, "no plugin dir provided, use default ./plugins")
		*pluginsDir = "plugins"
	}
	if *addr == "" {
		fmt.Fprintf(os.Stderr, "no addr provided, use default 127.0.0.1:8080")
		*addr = "127.0.0.1:8080"
	}
	if *layout == "" {
		fmt.Fprintf(os.Stderr, "no layout provided, use default layout.json")
		*layout = "layout.json"
	}
}
