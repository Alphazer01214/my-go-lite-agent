package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// defaultDir 相对插件进程的 CWD（Host 保证是插件目录）。
const defaultDir = "sessions"

type plugin struct {
	mu  sync.Mutex
	sdk *pluginsdk.Plugin
	dir string
	// reg 已打开文件句柄的 session（选中 workspace 的会话，或被按 id 访问的会话）
	reg map[string]*store
	// index 所有 session 的首行 meta，用于按 workspace 分类与懒加载
	index map[string]Meta
	// workspaces 持久化的 workspace 注册表
	workspaces *workspaceStore
}

func newPlugin() *plugin {
	return newPluginAtDir(defaultDir)
}

func newPluginAtDir(dir string) *plugin {
	if dir == "" {
		dir = defaultDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	workspaces, err := newWorkspaceStore(filepath.Join(dir, "workspaces.json"))
	if err != nil {
		panic(err)
	}
	p := &plugin{
		sdk:        pluginsdk.NewPlugin("session"),
		dir:        dir,
		reg:        make(map[string]*store),
		index:      make(map[string]Meta),
		workspaces: workspaces,
	}
	p.scanIndex()
	if selected, ok := workspaces.current(); ok {
		p.loadWorkspace(selected.WorkspaceId)
	}
	return p
}

func (p *plugin) sessionPath(id string) string {
	return filepath.Join(p.dir, id+".jsonl")
}

// scanIndex 只读每个会话文件的首行 meta，建立 workspace 分类索引；
// 不打开文件句柄，也不解析 facts。旧会话缺少 workspace_id 时按路径补全（仅内存）。
func (p *plugin) scanIndex() {
	ents, err := os.ReadDir(p.dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session: read dir %s: %v\n", p.dir, err)
		return
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		if id == "" {
			continue
		}
		meta, err := readFirstMeta(p.sessionPath(id))
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: index %s: %v\n", id, err)
			continue
		}
		if meta.SessionId == "" {
			meta.SessionId = id
		}
		if meta.WorkspaceId == "" && strings.TrimSpace(meta.Workspace) != "" {
			meta = p.bindWorkspaceMeta(meta)
		}
		p.index[id] = meta
	}
}

func (p *plugin) bindWorkspaceMeta(meta Meta) Meta {
	if record, ok := p.workspaces.findByPath(meta.Workspace); ok {
		meta.WorkspaceId = record.WorkspaceId
		meta.Workspace = record.Path
		return meta
	}
	record, _, err := p.workspaces.ensure(meta.Workspace, "", false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session: bind workspace for %s: %v\n", meta.SessionId, err)
		return meta
	}
	meta.WorkspaceId = record.WorkspaceId
	meta.Workspace = record.Path
	return meta
}

// readFirstMeta 只读取 JSONL 的首行 meta。
func readFirstMeta(path string) (Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if trimmed := strings.TrimSpace(string(line)); trimmed != "" {
			return decodeMeta(trimmed)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Meta{}, fmt.Errorf("no meta record")
			}
			return Meta{}, err
		}
	}
}

// loadWorkspace 打开该 workspace 下尚未加载的会话文件。
// 旧 workspace 的句柄不主动关闭：可能有调用方正持有 store，留到进程退出统一关闭。
func (p *plugin) loadWorkspace(workspaceId string) {
	if workspaceId == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, meta := range p.index {
		if meta.WorkspaceId != workspaceId || p.reg[id] != nil {
			continue
		}
		st, err := openStore(p.sessionPath(id), id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: load %s: %v\n", id, err)
			continue
		}
		if st.meta.WorkspaceId == "" && meta.WorkspaceId != "" {
			st.bindWorkspace(meta.WorkspaceId, meta.Workspace)
		}
		p.reg[id] = st
	}
}

// get 按需打开会话文件；会话可以不属于当前选中的 workspace。
func (p *plugin) get(id string) (*store, error) {
	if strings.TrimSpace(id) == "" {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "session_id is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if st := p.reg[id]; st != nil {
		return st, nil
	}
	meta, ok := p.index[id]
	if !ok {
		return nil, errCoded(codeSessionNotFound, "session not found: "+id)
	}
	st, err := openStore(p.sessionPath(id), id)
	if err != nil {
		return nil, errHandler(err)
	}
	if st.meta.WorkspaceId == "" && meta.WorkspaceId != "" {
		st.bindWorkspace(meta.WorkspaceId, meta.Workspace)
	}
	p.reg[id] = st
	return st, nil
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
		st.close()
	}
}
