package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type activeTurn struct {
	id     string
	cancel context.CancelFunc
}

type plugin struct {
	sdk      *pluginsdk.Plugin
	mu       sync.Mutex
	configMu sync.Mutex
	cfg      config
	turns    map[string]*activeTurn
	seq      atomic.Int64
}

func createPlugin() *plugin {
	cfg, err := loadConfig(agentConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: load config: %v\n", err)
		cfg = defaultConfig()
	}
	return &plugin{
		sdk:   pluginsdk.NewPlugin("agent"),
		cfg:   cfg,
		turns: make(map[string]*activeTurn),
	}
}
