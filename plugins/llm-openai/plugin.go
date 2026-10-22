package main

import (
	"context"
	"sync"
)

type plugin struct {
	mu     sync.Mutex
	cfg    config
	active map[string]context.CancelFunc
	// cancelled remembers cancels that arrived before their completion
	// registered. Host does not order req and cancel on the wire, so this is a
	// real case; cancelledOrder keeps the buffer bounded.
	cancelled      map[string]bool
	cancelledOrder []string
}

func newPlugin() *plugin {
	p := &plugin{active: make(map[string]context.CancelFunc), cancelled: make(map[string]bool)}
	p.cfg = loadConfig()
	return p
}
