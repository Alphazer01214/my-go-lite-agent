package main

import (
	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk *pluginsdk.Plugin
}

func createPlugin() *plugin {
	return &plugin{
		sdk: pluginsdk.NewPlugin("llm-openai"),
	}
}
