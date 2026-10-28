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
	p.sdk.Register(
		"file",
		"read",
		pluginsdk.NewHandler(handleJSON(p.handleread)).
			WithInfo("read", "read one workspace file"),
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
