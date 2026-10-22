package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (p *plugin) sessionPath(id string) (string, error) {
	if err := validSessionID(id); err != nil {
		return "", err
	}
	return filepath.Join(p.dir, id+".jsonl"), nil
}

func validSessionID(id string) error {
	if id == "" {
		return errBadArguments("session_id is required")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return errBadArguments("session_id must not contain a path separator")
	}
	return nil
}

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
	path, err := p.sessionPath(id)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
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
		if fact.Type == TypeSessionMeta && len(fact.Meta) > 0 {
			var meta Meta
			if err := json.Unmarshal(fact.Meta, &meta); err == nil {
				st.meta = meta
			}
		}
	}
	st.nextSeq = maxSeq + 1
	st.factCount = count
	st.lastTS = lastTS
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
