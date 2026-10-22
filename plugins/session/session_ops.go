package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func (p *plugin) create(ctx context.Context, in createIn) (createOut, error) {
	id := in.SessionID
	if id == "" {
		id = fmt.Sprintf("s-%d", time.Now().UnixNano())
	} else if err := validSessionID(id); err != nil {
		return createOut{}, err
	}
	p.mu.RLock()
	existing := p.reg[id]
	p.mu.RUnlock()
	if existing != nil {
		meta := existing.snapshotMeta()
		return createOut{SessionID: existing.id, CreatedTimestamp: meta.CreatedTimestamp}, nil
	}
	workspaceID, workspacePath, err := p.resolveWorkspaceBinding(in.WorkspaceID, in.Workspace)
	if err != nil {
		return createOut{}, err
	}
	p.mu.Lock()
	if st, ok := p.reg[id]; ok {
		p.mu.Unlock()
		meta := st.snapshotMeta()
		return createOut{SessionID: st.id, CreatedTimestamp: meta.CreatedTimestamp}, nil
	}

	contextWindow := in.ContextWindow
	if contextWindow < 0 {
		contextWindow = 0
	}
	meta := Meta{
		SessionID:        id,
		Name:             in.Name,
		WorkspaceID:      workspaceID,
		Workspace:        workspacePath,
		Model:            strings.TrimSpace(in.Model),
		ContextWindow:    contextWindow,
		ParentSessionID:  in.ParentSessionID,
		Origin:           in.Origin,
		DelegationDepth:  in.DelegationDepth,
		CreatedTimestamp: nowMS(),
	}
	path, err := p.sessionPath(id)
	if err != nil {
		p.mu.Unlock()
		return createOut{}, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
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

	p.publishAfterCreate(ctx, id)
	return createOut{SessionID: id, CreatedTimestamp: meta.CreatedTimestamp}, nil
}

func (p *plugin) append(ctx context.Context, in appendIn) (appendOut, error) {
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
	p.publishAfterAppend(ctx, st.id, appendedFacts(in.Facts, seqs, nowMS()), seqs[0]-1, seqs[len(seqs)-1])
	return appendOut{SessionID: st.id, Seqs: seqs, NextSeq: next}, nil
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
	out := queryOut{NextSeq: st.nextSequence(), Total: total}
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

func (p *plugin) readFacts(id string) ([]Fact, int, error) {
	path, err := p.sessionPath(id)
	if err != nil {
		return nil, 0, err
	}
	data, err := os.ReadFile(path)
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

func (p *plugin) list(workspaceID string, includeSubagents bool) (listOut, error) {
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
		meta := st.snapshotMeta()
		if workspaceID != "" && meta.WorkspaceID != workspaceID {
			continue
		}
		if !includeSubagents && meta.Origin == "subagent" {
			continue
		}
		count, lastTS := st.counts()
		out.Sessions = append(out.Sessions, sessionSummary{
			SessionID:        st.id,
			Name:             meta.Name,
			WorkspaceID:      meta.WorkspaceID,
			Workspace:        meta.Workspace,
			ParentSessionID:  meta.ParentSessionID,
			Origin:           meta.Origin,
			DelegationDepth:  meta.DelegationDepth,
			FactCount:        count,
			CreatedTimestamp: meta.CreatedTimestamp,
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
	meta := st.snapshotMeta()
	return infoOut{
		SessionID:        st.id,
		Name:             meta.Name,
		WorkspaceID:      meta.WorkspaceID,
		Workspace:        meta.Workspace,
		Model:            meta.Model,
		ContextWindow:    meta.ContextWindow,
		ParentSessionID:  meta.ParentSessionID,
		Origin:           meta.Origin,
		DelegationDepth:  meta.DelegationDepth,
		CreatedTimestamp: meta.CreatedTimestamp,
		FactCount:        total,
		NextSeq:          st.nextSequence(),
	}, nil
}
