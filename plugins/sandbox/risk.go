package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// UnknownRisk is the risk value used when the tool registry cannot answer.
// It sits in the write band on purpose: an unknown call is judged like a call
// that changes something.
const UnknownRisk = 70

// riskCacheTTL bounds how long a tool table is reused. The registry changes
// when a provider starts or stops, not on every call.
const riskCacheTTL = 5 * time.Second

// riskFailTTL bounds how long a failed lookup is reused. A failure is retried
// sooner than a success, so a transient outage does not deny calls for long.
const riskFailTTL = time.Second

// toolEntry is the slice of a tool definition the sandbox needs.
type toolEntry struct {
	Name             string `json:"name"`
	Risk             int    `json:"risk"`
	TargetCapability string `json:"target_capability"`
	TargetMethod     string `json:"target_method"`
}

// riskTable is the resolved tool registry, indexed by tool name and by
// target capability and method.
type riskTable struct {
	byName   map[string]toolEntry
	byTarget map[string]toolEntry
	err      string
	fetched  time.Time
}

// lookup finds a tool by name.
func (t riskTable) lookup(name string) (toolEntry, bool) {
	entry, ok := t.byName[name]
	return entry, ok
}

// lookupTarget finds a tool by the capability and method it targets.
func (t riskTable) lookupTarget(capability, method string) (toolEntry, bool) {
	entry, ok := t.byTarget[capability+"."+method]
	return entry, ok
}

// parseToolList reads a tools.list response into a table.
func parseToolList(raw json.RawMessage) (riskTable, error) {
	table := riskTable{
		byName:   map[string]toolEntry{},
		byTarget: map[string]toolEntry{},
		fetched:  time.Now(),
	}
	var out struct {
		Tools []toolEntry `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return table, err
	}
	for _, entry := range out.Tools {
		if entry.Name == "" {
			continue
		}
		table.byName[entry.Name] = entry
		if entry.TargetCapability != "" && entry.TargetMethod != "" {
			table.byTarget[entry.TargetCapability+"."+entry.TargetMethod] = entry
		}
	}
	return table, nil
}

// toolTable returns the tool registry, refreshing it when the cached copy is
// stale. A failing lookup is not fatal: the caller judges the call with the
// unknown risk value.
func (p *plugin) toolTable(ctx context.Context) riskTable {
	p.riskMu.Lock()
	defer p.riskMu.Unlock()
	ttl := riskCacheTTL
	if p.risk.err != "" {
		ttl = riskFailTTL
	}
	if !p.risk.fetched.IsZero() && time.Since(p.risk.fetched) < ttl {
		return p.risk
	}
	raw, err := p.sdk.Call(ctx, "tools", "list", json.RawMessage(`{}`))
	if err != nil {
		p.risk = riskTable{
			byName:   map[string]toolEntry{},
			byTarget: map[string]toolEntry{},
			err:      err.Error(),
			fetched:  time.Now(),
		}
		return p.risk
	}
	table, err := parseToolList(raw)
	if err != nil {
		p.risk = riskTable{
			byName:   map[string]toolEntry{},
			byTarget: map[string]toolEntry{},
			err:      err.Error(),
			fetched:  time.Now(),
		}
		return p.risk
	}
	p.risk = table
	return p.risk
}

// describe renders the table state for a log line.
func (t riskTable) describe() string {
	if t.err != "" {
		return "unavailable: " + t.err
	}
	return fmt.Sprintf("%d tool(s)", len(t.byName))
}
