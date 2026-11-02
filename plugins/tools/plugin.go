package main

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk               *pluginsdk.Plugin
	cfg               config
	configDiagnostics []diagnostic

	mu          sync.Mutex
	catalog     *catalog
	refreshing  bool
	refreshDone chan struct{}
	seq         atomic.Int64
}

func createPlugin() *plugin {
	cfg, diagnostics, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools: load config: %v\n", err)
	}
	for _, item := range diagnostics {
		fmt.Fprintf(os.Stderr, "tools: %s: %s\n", item.Code, item.Message)
	}
	return &plugin{
		sdk:               pluginsdk.NewPlugin("tools"),
		cfg:               cfg,
		configDiagnostics: diagnostics,
	}
}
