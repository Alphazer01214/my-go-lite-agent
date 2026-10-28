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
		sdk: pluginsdk.NewPlugin("sandbox"),
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"sandbox",
		"check",
		pluginsdk.NewHandler(handleJSON(p.handlecheck)).
			WithInfo("check", "check one sandbox request"),
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
