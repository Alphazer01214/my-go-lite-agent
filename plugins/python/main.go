package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type caller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	sdk              *pluginsdk.Plugin
	caller           caller
	interpreter      string
	interpreterFound bool
}

func newPlugin() *plugin {
	cfg := loadConfig()
	interpreter, found := resolveInterpreter(cfg.Interpreter, lookPathCommand)
	if !found {
		fmt.Fprintf(os.Stderr, "python: interpreter %q was not found; set interpreter in config.json if calls fail\n", interpreter)
	}
	sdk := pluginsdk.NewPlugin("python")
	return &plugin{
		sdk:              sdk,
		caller:           sdk,
		interpreter:      interpreter,
		interpreterFound: found,
	}
}

func (p *plugin) registerHandlers() {
	p.sdk.Register("python", "info", p.handleInfo)
	p.sdk.Register("python", "execute", p.handleExecute)
}

func main() {
	plugin := newPlugin()
	plugin.registerHandlers()

	if err := plugin.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
