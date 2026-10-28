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
		sdk: pluginsdk.NewPlugin("shell-tools"),
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"shell",
		"execute",
		pluginsdk.NewHandler(handleJSON(p.handleexecute)).
			WithInfo("execute", "execute one shell request"),
	)
}

func main() {
	plugin := newPlugin()
	plugin.registerHandlers()

	if err := plugin.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
