package main

import (
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type llmPlugin struct {
	sdk *pluginsdk.Plugin
}

func newLLMPlugin() *llmPlugin {
	return &llmPlugin{
		sdk: pluginsdk.NewPlugin("llm-openai"),
	}
}

func (p *llmPlugin) registerHandlers() {
	p.sdk.Register(
		"llm",
		"complete",
		pluginsdk.NewHandler(handleJSON(p.handleComplete)).
			WithInfo("complete", "complete one model request"),
	)
}

func main() {
	plugin := newLLMPlugin()
	plugin.registerHandlers()

	if err := plugin.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
