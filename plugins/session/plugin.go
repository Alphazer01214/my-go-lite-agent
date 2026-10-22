package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	dir                 string
	workspaces          *workspaceStore
	sdk                 sdkCaller
	cfg                 config
	mu                  sync.RWMutex
	reg                 map[string]*store
	streams             map[string]streamSnapshot
	streamOutMu         sync.Mutex
	streamOut           map[string]*streamOutDelta
	streamOnce          sync.Once
	streamStop          chan struct{}
	streamStopOnce      sync.Once
	ui                  uiPublisher
	uiRevision          uint64
	selected            string
	open                []string
	selectedByWorkspace map[string]string
}

const defaultDir = "./sessions"

func dataDir() string {
	if d := os.Getenv("SESSION_DATA_DIR"); d != "" {
		return d
	}
	return defaultDir
}

func newPlugin(dir string) *plugin {
	if dir == "" {
		dir = defaultDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	workspaces, err := newWorkspaceStore(workspaceDataFile(dir), legacyWorkspaceDataFile(dir))
	if err != nil {
		panic(err)
	}
	return &plugin{
		dir: dir, workspaces: workspaces, reg: make(map[string]*store), streams: make(map[string]streamSnapshot),
		streamOut: make(map[string]*streamOutDelta), streamStop: make(chan struct{}),
		cfg: loadConfig(), selectedByWorkspace: make(map[string]string),
		// Revisions must stay monotonic across process restarts. A browser that
		// reconnects to a restarted plugin would otherwise discard the fresh
		// state as stale because the counter reset to zero.
		uiRevision: uint64(time.Now().UnixMicro()),
	}
}

func loadConfig() config {
	cfg := defaultConfig()
	data, err := os.ReadFile("config.json")
	if err != nil {
		return cfg
	}
	var raw struct {
		FullToolResults *int `json:"fullToolResults"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg
	}
	if raw.FullToolResults != nil && *raw.FullToolResults >= 0 {
		cfg.FullToolResults = *raw.FullToolResults
	}
	return cfg
}

func (p *plugin) close() {
	p.streamStopOnce.Do(func() { close(p.streamStop) })
	p.mu.Lock()
	stores := make([]*store, 0, len(p.reg))
	for _, st := range p.reg {
		stores = append(stores, st)
	}
	p.reg = make(map[string]*store)
	p.mu.Unlock()
	for _, st := range stores {
		st.mu.Lock()
		if st.f != nil {
			_ = st.f.Close()
			st.f = nil
		}
		st.mu.Unlock()
	}
}
