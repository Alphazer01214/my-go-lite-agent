package main

import (
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk *pluginsdk.Plugin
	cfg config
}

func createPlugin() *plugin {
	cfg, err := loadConfig()
	if err != nil {
		logf("load config: %v", err)
		os.Exit(1)
	}
	return &plugin{
		sdk: pluginsdk.NewPlugin("prompter"),
		cfg: cfg,
	}
}
