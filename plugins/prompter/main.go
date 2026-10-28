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
		sdk: pluginsdk.NewPlugin("prompter"),
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"system-prompt",
		"build",
		pluginsdk.NewHandler(handleJSON(p.handlebuild)).
			WithInfo("build", "build one system prompt"),
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
