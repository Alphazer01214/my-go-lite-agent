package main

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type activeTurn struct {
	id     string
	cancel context.CancelFunc
}

type plugin struct {
	sdk   *pluginsdk.Plugin
	mu    sync.Mutex
	turns map[string]*activeTurn
	seq   atomic.Int64
}

func createPlugin() *plugin {
	return &plugin{
		sdk:   pluginsdk.NewPlugin("agent"),
		turns: make(map[string]*activeTurn),
	}
}
