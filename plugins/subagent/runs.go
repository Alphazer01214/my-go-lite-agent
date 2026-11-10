package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type runMeta struct {
	ChildSessionID  string `json:"child_session_id"`
	ParentSessionID string `json:"parent_session_id"`
	State           string `json:"state"`
	DelegationDepth int    `json:"delegation_depth"`
	StartedTs       int64  `json:"started_ts"`
	DurationMs      int64  `json:"duration_ms"`
	TurnCount       int    `json:"turn_count"`
	Error           string `json:"error"`
}

type runAccumulator struct {
	run      runView
	firstSeq int
}

func (p *plugin) listRuns(ctx context.Context, contextID, sessionID string) (runsResp, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return runsResp{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "session_id is required")
	}
	raw, err := p.call(ctx, contextID, "session", "facts", factsReq{
		SessionID: sessionID,
		Types:     []string{factTypeSpawned, factTypeFinished},
	})
	if err != nil {
		return runsResp{}, err
	}
	var out factsResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return runsResp{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "bad session.facts response: "+err.Error())
	}

	order := make([]string, 0, len(out.Facts))
	byChild := make(map[string]*runAccumulator, len(out.Facts))
	for _, fact := range out.Facts {
		var meta runMeta
		if len(fact.Meta) > 0 {
			_ = json.Unmarshal(fact.Meta, &meta)
		}
		childID := strings.TrimSpace(meta.ChildSessionID)
		if childID == "" {
			continue
		}
		entry, ok := byChild[childID]
		if !ok {
			entry = &runAccumulator{
				run: runView{
					ChildSessionID:  childID,
					State:           "running",
					DelegationDepth: meta.DelegationDepth,
					StartedTs:       meta.StartedTs,
				},
				firstSeq: fact.Seq,
			}
			if entry.run.StartedTs == 0 {
				entry.run.StartedTs = fact.Timestamp
			}
			byChild[childID] = entry
			order = append(order, childID)
		}
		if entry.run.DelegationDepth == 0 {
			entry.run.DelegationDepth = meta.DelegationDepth
		}
		if fact.Type != factTypeFinished {
			continue
		}
		state := strings.TrimSpace(meta.State)
		if state == "" {
			state = "completed"
		}
		entry.run.State = state
		entry.run.DurationMs = meta.DurationMs
		entry.run.TurnCount = meta.TurnCount
		entry.run.Error = strings.TrimSpace(meta.Error)
		entry.run.FinishedTs = fact.Timestamp
	}

	runs := make([]runView, 0, len(order))
	for _, childID := range order {
		run := byChild[childID].run
		if run.DurationMs == 0 && run.FinishedTs > run.StartedTs && run.StartedTs > 0 {
			run.DurationMs = run.FinishedTs - run.StartedTs
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].StartedTs != runs[j].StartedTs {
			return runs[i].StartedTs > runs[j].StartedTs
		}
		return byChild[runs[i].ChildSessionID].firstSeq < byChild[runs[j].ChildSessionID].firstSeq
	})
	for index := range runs {
		p.enrichRun(ctx, contextID, &runs[index])
	}
	return runsResp{SessionID: sessionID, Runs: runs}, nil
}

func (p *plugin) enrichRun(ctx context.Context, contextID string, run *runView) {
	if run == nil || run.ChildSessionID == "" {
		return
	}
	info, err := p.sessionInfo(ctx, contextID, run.ChildSessionID)
	if err != nil {
		return
	}
	run.Title = strings.TrimSpace(info.Title)
	run.FactCount = info.FactCount
	run.MessageCount = info.MessageCount
}

// publishRunsState nudges this plugin's UI so a visible panel can reload while
// its parent turn is still streaming.
func (p *plugin) publishRunsState(sessionID, state string) {
	type uiPublisher interface {
		WebUI() *pluginsdk.WebUI
	}
	publisher, ok := p.sdk.(uiPublisher)
	if !ok {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": sessionID,
		"state":      state,
		"updated_at": time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	go func() {
		_ = publisher.WebUI().SetState("subagent", payload)
	}()
}
