package main

import (
	"fmt"
	"os"
)

func (p *plugin) registerHandlers() {
	p.sdk.Register("tool", "list", p.handleList)
	p.sdk.Register("tool", "call", p.handleCall)
}

func main() {
	p := createPlugin()
	p.registerHandlers()

	if err := p.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
