package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk    *pluginsdk.Plugin
	cfg    config
	client *http.Client
}

func createPlugin() *plugin {
	cfg, err := loadConfig("config.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm-openai: load config: %v\n", err)
	}
	return &plugin{
		sdk:    pluginsdk.NewPlugin("llm-openai"),
		cfg:    cfg,
		client: &http.Client{},
	}
}
