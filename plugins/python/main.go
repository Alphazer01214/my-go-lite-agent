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
		sdk: pluginsdk.NewPlugin("python"),
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"python",
		"run",
		pluginsdk.NewHandler(handleJSON(p.handlerun)).
			WithInfo("run", "run one python request"),
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
