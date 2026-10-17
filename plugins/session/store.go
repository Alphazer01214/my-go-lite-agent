package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type store struct {
	mu        sync.Mutex
	id        string
	meta      Meta
	f         *os.File
	nextSeq   int
	factCount int
	lastTS    int64
}

type sdkCaller interface {
	Call(capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	dir        string
	sdk        sdkCaller
	cfg        config
	mu         sync.RWMutex
	reg        map[string]*store
	streams    map[string]streamSnapshot
	ui         uiPublisher
	uiRevision uint64
	selected   string
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
	return &plugin{dir: dir, reg: make(map[string]*store), streams: make(map[string]streamSnapshot), cfg: loadConfig()}
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

func (p *plugin) sessionPath(id string) string {
	return filepath.Join(p.dir, id+".jsonl")
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// loadAll scans *.jsonl into reg. Corrupt lines are skipped with a warning.
func (p *plugin) loadAll() {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session: read dir %s: %v\n", p.dir, err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		if id == "" {
			continue
		}
		st, err := p.recover(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: load %s: %v\n", id, err)
			continue
		}
		p.reg[id] = st
	}
}

func (p *plugin) recover(id string) (*store, error) {
	path := p.sessionPath(id)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	st := &store{id: id, f: f, nextSeq: 1}
	data, err := os.ReadFile(path)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	maxSeq := 0
	count := 0
	var lastTS int64
	openTurns := make(map[string]map[string]any)
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var fact Fact
		if err := json.Unmarshal([]byte(line), &fact); err != nil {
			fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", id, i+1, err)
			continue
		}
		count++
		if fact.Seq > maxSeq {
			maxSeq = fact.Seq
		}
		if fact.Timestamp > lastTS {
			lastTS = fact.Timestamp
		}
		if fact.Type == TypeTurnStart {
			meta := map[string]any{}
			_ = json.Unmarshal(fact.Meta, &meta)
			turnID, _ := meta["turn_id"].(string)
			if turnID == "" {
				turnID = fmt.Sprintf("recovered-%d", fact.Seq)
				meta["turn_id"] = turnID
			}
			openTurns[turnID] = meta
		} else if fact.Type == TypeTurnEnd {
			meta := map[string]any{}
			_ = json.Unmarshal(fact.Meta, &meta)
			if turnID, _ := meta["turn_id"].(string); turnID != "" {
				delete(openTurns, turnID)
			}
		}
		if fact.Type == TypeSessionStart && len(fact.Meta) > 0 {
			var meta Meta
			if err := json.Unmarshal(fact.Meta, &meta); err == nil {
				st.meta = meta
			}
		}
	}
	if len(openTurns) > 0 {
		turnIDs := make([]string, 0, len(openTurns))
		for turnID := range openTurns {
			turnIDs = append(turnIDs, turnID)
		}
		sort.Strings(turnIDs)
		recovered := make([]factIn, 0, len(turnIDs))
		for _, turnID := range turnIDs {
			meta := openTurns[turnID]
			meta["reason"] = "error"
			meta["error"] = "agent exited before turn_end"
			recovered = append(recovered, factIn{
				Type: TypeTurnEnd,
				Role: RoleHost,
				Meta: mustJSON(meta),
			})
		}
		if _, _, err := st.appendBatch(recovered); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	st.nextSeq = maxSeq + 1
	st.factCount = count
	st.lastTS = lastTS
	if st.meta.SessionID == "" {
		st.meta.SessionID = id
	}
	if st.meta.CreatedTimestamp == 0 {
		if fi, err := os.Stat(path); err == nil {
			st.meta.CreatedTimestamp = fi.ModTime().UnixMilli()
		}
	}
	return st, nil
}

func nowMS() int64 { return time.Now().UnixMilli() }

func (p *plugin) get(id string) (*store, error) {
	if id == "" {
		return nil, errBadArguments("session_id is required")
	}
	p.mu.RLock()
	st, ok := p.reg[id]
	p.mu.RUnlock()
	if !ok {
		return nil, errNotFound(id)
	}
	return st, nil
}

func errBadArguments(msg string) error {
	return errCoded("bad_arguments", msg)
}

func errNotFound(id string) error {
	return errCoded("session_not_found", "session not found: "+id)
}

func errHandler(msg string) error {
	return errCoded("handler_error", msg)
}

// create is agent-explicit only; never called from append. Idempotent.
func (p *plugin) create(in createIn) (createOut, error) {
	id := in.SessionID
	if id == "" {
		id = fmt.Sprintf("s-%d", time.Now().UnixNano())
	}
	p.mu.Lock()
	if st, ok := p.reg[id]; ok {
		p.mu.Unlock()
		return createOut{SessionID: st.id, CreatedTimestamp: st.meta.CreatedTimestamp}, nil
	}

	meta := Meta{
		SessionID:        id,
		Name:             in.Name,
		Workspace:        in.Workspace,
		ParentSessionID:  in.ParentSessionID,
		Origin:           in.Origin,
		DelegationDepth:  in.DelegationDepth,
		CreatedTimestamp: nowMS(),
	}
	f, err := os.OpenFile(p.sessionPath(id), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		p.mu.Unlock()
		return createOut{}, errHandler(err.Error())
	}
	st := &store{id: id, meta: meta, f: f, nextSeq: 1}
	if _, _, err := st.appendLocked([]factIn{{
		Type: TypeSessionStart,
		Role: RoleHost,
		Meta: mustJSON(meta),
	}}); err != nil {
		_ = f.Close()
		p.mu.Unlock()
		return createOut{}, err
	}
	p.reg[id] = st
	p.mu.Unlock()

	p.publishAfterCreate(id)
	return createOut{SessionID: id, CreatedTimestamp: meta.CreatedTimestamp}, nil
}

// append never implicitly creates a session.
func (p *plugin) append(in appendIn) (appendOut, error) {
	if len(in.Facts) == 0 {
		return appendOut{}, errBadArguments("facts must be non-empty")
	}
	for _, f := range in.Facts {
		if f.Type == "" {
			return appendOut{}, errBadArguments("fact.type is required")
		}
	}
	st, err := p.get(in.SessionID)
	if err != nil {
		return appendOut{}, err
	}
	seqs, next, err := st.appendBatch(in.Facts)
	if err != nil {
		return appendOut{}, err
	}
	for _, fact := range in.Facts {
		if fact.Type != TypeMessage && fact.Type != TypeTurnEnd {
			continue
		}
		meta := p.sessionFactContext(fact.Meta)
		if fact.Type == TypeTurnEnd || (fact.Role == RoleAssistant && meta.TurnID != "") {
			p.clearStream(st.id, meta.TurnID, meta.StepID)
		}
	}
	p.publishAfterAppend(st.id)
	return appendOut{SessionID: st.id, Seqs: seqs, NextSeq: next}, nil
}

func (s *store) appendBatch(ins []factIn) ([]int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(ins)
}

func (s *store) appendLocked(ins []factIn) ([]int, int, error) {
	ts := nowMS()
	seqs := make([]int, 0, len(ins))
	facts := make([]Fact, 0, len(ins))
	for _, in := range ins {
		seq := s.nextSeq
		s.nextSeq++
		seqs = append(seqs, seq)
		facts = append(facts, Fact{
			Seq:        seq,
			Type:       in.Type,
			Role:       in.Role,
			Content:    in.Content,
			ToolCalls:  in.ToolCalls,
			ToolCallID: in.ToolCallID,
			Meta:       in.Meta,
			Timestamp:  ts,
		})
	}
	var b strings.Builder
	for _, f := range facts {
		line, err := json.Marshal(f)
		if err != nil {
			return nil, s.nextSeq, errHandler(err.Error())
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if _, err := s.f.WriteString(b.String()); err != nil {
		return nil, s.nextSeq, errHandler(err.Error())
	}
	s.factCount += len(ins)
	s.lastTS = ts
	return seqs, s.nextSeq, nil
}

func (s *store) counts() (int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.factCount, s.lastTS
}

func (p *plugin) query(in queryIn) (queryOut, error) {
	st, err := p.get(in.SessionID)
	if err != nil {
		return queryOut{}, err
	}
	facts, total, err := p.readFacts(st.id)
	if err != nil {
		return queryOut{}, err
	}
	out := queryOut{NextSeq: st.nextSeq, Total: total}
	typeSet := map[string]bool{}
	for _, t := range in.Types {
		typeSet[t] = true
	}
	for _, f := range facts {
		if f.Seq <= in.AfterSeq {
			continue
		}
		if len(typeSet) > 0 && !typeSet[f.Type] {
			continue
		}
		out.Facts = append(out.Facts, f)
		if in.Limit > 0 && len(out.Facts) >= in.Limit {
			break
		}
	}
	if out.Facts == nil {
		out.Facts = []Fact{}
	}
	return out, nil
}

// readFacts scans the jsonl file (soft-skips corrupt lines).
func (p *plugin) readFacts(id string) ([]Fact, int, error) {
	data, err := os.ReadFile(p.sessionPath(id))
	if err != nil {
		return nil, 0, errHandler(err.Error())
	}
	var facts []Fact
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f Fact
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", id, i+1, err)
			continue
		}
		facts = append(facts, f)
	}
	return facts, len(facts), nil
}

func (p *plugin) derive(in deriveIn) (deriveOut, error) {
	st, err := p.get(in.SessionID)
	if err != nil {
		return deriveOut{}, err
	}
	full := p.cfg.FullToolResults
	if in.FullToolResults != nil {
		full = *in.FullToolResults
	}
	facts, _, err := p.readFacts(st.id)
	if err != nil {
		return deriveOut{}, err
	}
	return projectMessages(facts, full), nil
}

func (p *plugin) list() (listOut, error) {
	p.mu.RLock()
	ids := make([]string, 0, len(p.reg))
	for id := range p.reg {
		ids = append(ids, id)
	}
	p.mu.RUnlock()

	out := listOut{Sessions: make([]sessionSummary, 0, len(ids))}
	for _, id := range ids {
		st, err := p.get(id)
		if err != nil {
			continue
		}
		count, lastTS := st.counts()
		out.Sessions = append(out.Sessions, sessionSummary{
			SessionID:        st.id,
			Name:             st.meta.Name,
			Workspace:        st.meta.Workspace,
			FactCount:        count,
			CreatedTimestamp: st.meta.CreatedTimestamp,
			LastTimestamp:    lastTS,
		})
	}
	// Newest first. Map iteration is randomized, so the order must be explicit.
	sort.Slice(out.Sessions, func(i, j int) bool {
		a, b := out.Sessions[i], out.Sessions[j]
		if a.LastTimestamp != b.LastTimestamp {
			return a.LastTimestamp > b.LastTimestamp
		}
		if a.CreatedTimestamp != b.CreatedTimestamp {
			return a.CreatedTimestamp > b.CreatedTimestamp
		}
		return a.SessionID > b.SessionID
	})
	return out, nil
}

func (p *plugin) info(in infoIn) (infoOut, error) {
	st, err := p.get(in.SessionID)
	if err != nil {
		return infoOut{}, err
	}
	_, total, err := p.readFacts(st.id)
	if err != nil {
		return infoOut{}, err
	}
	return infoOut{
		SessionID:        st.id,
		Name:             st.meta.Name,
		Workspace:        st.meta.Workspace,
		ParentSessionID:  st.meta.ParentSessionID,
		Origin:           st.meta.Origin,
		DelegationDepth:  st.meta.DelegationDepth,
		CreatedTimestamp: st.meta.CreatedTimestamp,
		FactCount:        total,
		NextSeq:          st.nextSeq,
	}, nil
}
