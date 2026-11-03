package main

import (
	"os"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type plugin struct {
	sdk        *pluginsdk.Plugin
	configMu   sync.RWMutex
	cfg        config
	configPath string

	contextMu   sync.Mutex
	contexts    map[string]contextCacheEntry
	lastContext string
	contextSeq  uint64

	skillMu        sync.Mutex
	skillKey       string
	skillSkills    []skillInfo
	skillErrors    []string
	skillScannedAt time.Time

	mcp *mcpManager
}

func createPlugin() *plugin {
	cfg, err := loadConfig(configPath)
	if err != nil {
		logf("load config: %v", err)
		os.Exit(1)
	}
	return &plugin{
		sdk:        pluginsdk.NewPlugin("prompter"),
		cfg:        cfg,
		configPath: configPath,
		contexts:   make(map[string]contextCacheEntry),
	}
}

func (p *plugin) currentConfig() config {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.cfg
}
