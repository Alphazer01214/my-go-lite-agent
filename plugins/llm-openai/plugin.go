package main

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	mu     sync.Mutex
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
