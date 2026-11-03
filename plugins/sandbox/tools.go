package main

import (
	"context"
	"encoding/json"
	"time"
)

const (
	toolIndexTTL = 5 * time.Second
	toolFailTTL  = time.Second
)

type toolEntry struct {
	Name       string `json:"name"`
	Capability string `json:"capability"`
	Method     string `json:"method"`
}

type toolIndex struct {
	byName   map[string]toolEntry
	byTarget map[string]toolEntry
	err      string
	fetched  time.Time
}

func (t toolIndex) lookup(name string) (toolEntry, bool) {
	entry, ok := t.byName[name]
	return entry, ok
}

func (t toolIndex) lookupTarget(capability, method string) (toolEntry, bool) {
	entry, ok := t.byTarget[capability+"."+method]
	return entry, ok
}

func parseToolList(raw json.RawMessage) (toolIndex, error) {
	index := toolIndex{
		byName:   map[string]toolEntry{},
		byTarget: map[string]toolEntry{},
		fetched:  time.Now(),
	}
	var out struct {
		Tools []toolEntry `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return index, err
	}
	for _, entry := range out.Tools {
		if entry.Name == "" {
			continue
		}
		index.byName[entry.Name] = entry
		if entry.Capability != "" && entry.Method != "" {
			index.byTarget[entry.Capability+"."+entry.Method] = entry
		}
	}
	return index, nil
}

func (p *plugin) toolIndex(ctx context.Context) toolIndex {
	p.toolMu.Lock()
	defer p.toolMu.Unlock()
	ttl := toolIndexTTL
	if p.tools.err != "" {
		ttl = toolFailTTL
	}
	if !p.tools.fetched.IsZero() && time.Since(p.tools.fetched) < ttl {
		return p.tools
	}
	raw, err := p.sdk.Call(ctx, "tool", "list", json.RawMessage(`{}`))
	if err != nil {
		p.tools = toolIndex{
			byName:   map[string]toolEntry{},
			byTarget: map[string]toolEntry{},
			err:      err.Error(),
			fetched:  time.Now(),
		}
		return p.tools
	}
	index, err := parseToolList(raw)
	if err != nil {
		p.tools = toolIndex{
			byName:   map[string]toolEntry{},
			byTarget: map[string]toolEntry{},
			err:      err.Error(),
			fetched:  time.Now(),
		}
		return p.tools
	}
	p.tools = index
	return p.tools
}
