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
		sdk: pluginsdk.NewPlugin("tools"),
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"tools",
		"list",
		pluginsdk.NewHandler(handleJSON(p.handlelist)).
			WithInfo("list", "list registered tools"),
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
