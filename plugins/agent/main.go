package main

import (
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type Agent struct {
	sdk       *pluginsdk.Plugin
	workspace string
}

func newAgent() *Agent {
	return &Agent{
		sdk: pluginsdk.NewPlugin("agent"),
	}
}

func (a *Agent) registerHandlers() {
	a.sdk.Register(
		"loop",
		"turn",
		pluginsdk.NewHandler(handleJSON(a.handleTurn)).
			WithInfo("turn", "run one agent turn"),
	)
}

func main() {
	agent := newAgent()
	agent.registerHandlers()

	if err := agent.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
