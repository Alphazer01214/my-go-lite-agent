package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// create 新建会话文件：首行写入 meta，再把 store 注册到内存。
func (p *plugin) create(ctx context.Context, req *createReq) (*Meta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspace, err := p.resolveWorkspace(req.WorkspaceId, req.Workspace)
	if err != nil {
		return nil, err
	}
	origin := strings.TrimSpace(req.Origin)
	if origin == "" {
		origin = "agent"
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for attempt := 0; attempt < 4; attempt++ {
		id, err := newSessionID()
		if err != nil {
			return nil, errHandler(err)
		}
		if p.reg[id] != nil {
			continue
		}
		meta := Meta{
			SessionId:       id,
			Title:           req.Title,
			WorkspaceId:     workspace.WorkspaceId,
			Workspace:       workspace.Path,
			ParentSessionId: req.ParentSessionID,
			Origin:          origin,
			CreatedAt:       time.Now().UnixMilli(),
		}
		st, err := createStore(p.sessionPath(id), id, meta)
		if err != nil {
			if os.IsExist(err) {
				continue
			}
			return nil, errHandler(err)
		}
		p.reg[id] = st
		p.index[id] = meta
		return &meta, nil
	}
	return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "could not allocate a unique session id")
}

// newSessionID 用毫秒时间戳加随机后缀，避免并行创建时撞 ID。
func newSessionID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("s-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:])), nil
}

func (p *plugin) info(ctx context.Context, req *infoReq) (*infoResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, err := p.get(req.SessionId)
	if err != nil {
		return nil, err
	}
	meta, factCnt, nextSeq, _ := st.snapshot()
	return &infoResp{Meta: meta, FactCnt: factCnt, NextSeq: nextSeq}, nil
}

func (p *plugin) appendFacts(ctx context.Context, req *appendReq) (*appendResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(req.Facts) == 0 {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "facts must be non-empty")
	}
	for i := range req.Facts {
		if strings.TrimSpace(req.Facts[i].Type) == "" {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "fact.type is required")
		}
	}
	st, err := p.get(req.SessionId)
	if err != nil {
		return nil, err
	}
	seqs, nextSeq, err := st.appendBatch(req.Facts)
	if err != nil {
		return nil, errHandler(err)
	}
	return &appendResp{SessionId: st.id, Seqs: seqs, NextSeq: nextSeq}, nil
}

func (p *plugin) facts(ctx context.Context, req *factsReq) (*factsResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, err := p.get(req.SessionId)
	if err != nil {
		return nil, err
	}
	facts, nextSeq, err := st.readFacts()
	if err != nil {
		return nil, errHandler(err)
	}

	typeSet := make(map[string]bool, len(req.Types))
	for _, t := range req.Types {
		typeSet[t] = true
	}
	out := factsResp{Facts: []Fact{}, NextSeq: nextSeq, Total: len(facts)}
	for _, f := range facts {
		if f.Seq <= req.AfterSeq {
			continue
		}
		if len(typeSet) > 0 && !typeSet[f.Type] {
			continue
		}
		out.Facts = append(out.Facts, f)
		if req.Limit > 0 && len(out.Facts) >= req.Limit {
			break
		}
	}
	return &out, nil
}

// listSessions 返回指定 workspace 的会话；未指定时使用当前选中项。
// 选中 workspace 的会话文件在返回前已按需打开，因此计数是完整的。
func (p *plugin) listSessions(ctx context.Context, req *listReq) (*listResp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaceId := strings.TrimSpace(req.WorkspaceId)
	if workspaceId == "" {
		selected, ok := p.workspaces.current()
		if !ok {
			return &listResp{Sessions: []sessionSummary{}}, nil
		}
		workspaceId = selected.WorkspaceId
	} else if _, ok := p.workspaces.info(workspaceId); !ok {
		return nil, errCoded(codeWorkspaceNotFound, "workspace not found: "+workspaceId)
	}

	p.loadWorkspace(workspaceId)

	p.mu.Lock()
	out := make([]sessionSummary, 0)
	for id, meta := range p.index {
		if meta.WorkspaceId != workspaceId {
			continue
		}
		if !req.IncludeSubagents && meta.Origin == "subagent" {
			continue
		}
		summary := sessionSummary{Meta: meta}
		if st := p.reg[id]; st != nil {
			current, factCnt, nextSeq, lastTs := st.snapshot()
			summary.Meta = current
			summary.FactCnt = factCnt
			summary.NextSeq = nextSeq
			summary.LastTs = lastTs
		}
		out = append(out, summary)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if a.LastTs != b.LastTs {
			return a.LastTs > b.LastTs
		}
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt > b.CreatedAt
		}
		return a.SessionId > b.SessionId
	})
	return &listResp{WorkspaceId: workspaceId, Sessions: out}, nil
}

func errHandler(err error) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
}
