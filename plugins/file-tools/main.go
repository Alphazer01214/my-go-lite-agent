package main

import (
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk *pluginsdk.Plugin
}

func newPlugin() *plugin {
	return &plugin{
		sdk: pluginsdk.NewPlugin("file-tools"),
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register("file", "info", p.handleToolInfo)
	p.sdk.Register("file", "read", p.handleRead)
	p.sdk.Register("file", "write", p.handleWrite)
	p.sdk.Register("file", "glob", p.handleGlob)
	p.sdk.Register("file", "grep", p.handleGrep)
	p.sdk.Register("file", "edit", p.handleEdit)
}

func main() {
	plugin := newPlugin()
	plugin.registerHandlers()

	if err := plugin.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
