package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const workspaceRegistryVersion = 2

// workspaceRegistryState 的持久化 schema；v2 起以 path 为唯一标识，不再存 id。
// 旧版（v1）注册表带 workspace_id/selected_id，加载时由 migrateRegistry 迁移，
// 因此 dist 里已有的注册表与 build 脚本的搬迁逻辑可以直接接上。
type workspaceRegistryState struct {
	Version    int         `json:"version"`
	Selected   string      `json:"selected,omitempty"`
	Workspaces []Workspace `json:"workspaces"`
}

// workspaceRegistryFile 是 v1→v2 迁移用的宽松解码结构，保留旧的 id 字段。
type workspaceRegistryFile struct {
	Version    int    `json:"version"`
	Selected   string `json:"selected"`
	SelectedId string `json:"selected_id"`
	Workspaces []struct {
		WorkspaceId string `json:"workspace_id"`
		Name        string `json:"name"`
		Path        string `json:"path"`
		CreatedAt   int64  `json:"created_ts"`
		UpdatedAt   int64  `json:"updated_ts"`
	} `json:"workspaces"`
}

// migrateRegistry 把 v1 注册表（带 workspace_id/selected_id）迁到 v2：
// 丢弃 id 字段，选中项从 id 映射回 path。
func migrateRegistry(file workspaceRegistryFile) workspaceRegistryState {
	state := workspaceRegistryState{
		Version:    workspaceRegistryVersion,
		Selected:   file.Selected,
		Workspaces: []Workspace{},
	}
	for _, item := range file.Workspaces {
		state.Workspaces = append(state.Workspaces, Workspace{
			Name:      item.Name,
			Path:      item.Path,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
		if item.WorkspaceId != "" && item.WorkspaceId == file.SelectedId {
			state.Selected = item.Path
		}
	}
	return state
}

type workspaceStore struct {
	mu    sync.Mutex
	path  string
	state workspaceRegistryState
}

func newWorkspaceStore(path string) (*workspaceStore, error) {
	store := &workspaceStore{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		store.state = workspaceRegistryState{
			Version: workspaceRegistryVersion, Workspaces: []Workspace{},
		}
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var file workspaceRegistryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode workspace registry: %w", err)
	}
	if file.Version > workspaceRegistryVersion {
		return nil, fmt.Errorf("unsupported workspace registry version %d", file.Version)
	}
	store.state = migrateRegistry(file)
	return store, nil
}

func (s *workspaceStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".workspaces-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *workspaceStore) findPathLocked(path string) int {
	key := workspacePathKey(path)
	for i, record := range s.state.Workspaces {
		if workspacePathKey(record.Path) == key {
			return i
		}
	}
	return -1
}

// list 返回副本与当前选中，置顶语义由 UpdatedAt 决定。
func (s *workspaceStore) list() ([]Workspace, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]Workspace{}, s.state.Workspaces...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return items[i].UpdatedAt > items[j].UpdatedAt
		}
		return items[i].Path > items[j].Path
	})
	return items, s.state.Selected
}

func (s *workspaceStore) info(path string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findPathLocked(path)
	if index < 0 {
		return Workspace{}, false
	}
	return s.state.Workspaces[index], true
}

func (s *workspaceStore) current() (*Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findPathLocked(s.state.Selected)
	if index < 0 {
		return nil, false
	}
	record := s.state.Workspaces[index]
	return &record, true
}

func (s *workspaceStore) selectedPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Selected
}

// findByPath 按路径匹配已有记录；路径无法规范化时退回清理后的绝对路径比较。
func (s *workspaceStore) findByPath(path string) (Workspace, bool) {
	key := workspacePathKeyOf(path)
	if key == "" {
		return Workspace{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.state.Workspaces {
		if workspacePathKey(record.Path) == key {
			return record, true
		}
	}
	return Workspace{}, false
}

// ensure 注册一个已存在的目录。路径重复时返回已有记录；
// selectWorkspace 为 true 或当前没有选中项时，选中该 workspace。
func (s *workspaceStore) ensure(path, name string, selectWorkspace bool) (Workspace, bool, error) {
	canonical, err := canonicalWorkspacePath(path)
	if err != nil {
		return Workspace{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if index := s.findPathLocked(canonical); index >= 0 {
		record := s.state.Workspaces[index]
		if selectWorkspace && s.state.Selected != record.Path {
			previous := cloneWorkspaceState(s.state)
			record.UpdatedAt = time.Now().UnixMilli()
			s.state.Selected = record.Path
			s.state.Workspaces = append([]Workspace(nil), s.state.Workspaces...)
			s.state.Workspaces[index] = record
			if err := s.saveLocked(); err != nil {
				s.state = previous
				return Workspace{}, false, err
			}
		}
		return record, false, nil
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultWorkspaceName(canonical)
	}
	now := time.Now().UnixMilli()
	record := Workspace{
		Name:      name,
		Path:      canonical,
		CreatedAt: now,
		UpdatedAt: now,
	}
	previous := cloneWorkspaceState(s.state)
	s.state.Workspaces = append(s.state.Workspaces, record)
	if selectWorkspace || s.state.Selected == "" {
		s.state.Selected = record.Path
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return Workspace{}, false, err
	}
	return record, true, nil
}

func (s *workspaceStore) selectPath(path string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findPathLocked(path)
	if index < 0 {
		return Workspace{}, errors.New("workspace not found")
	}
	record := s.state.Workspaces[index]
	if s.state.Selected == record.Path {
		return record, nil
	}
	previous := cloneWorkspaceState(s.state)
	record.UpdatedAt = time.Now().UnixMilli()
	s.state.Selected = record.Path
	s.state.Workspaces = append([]Workspace(nil), s.state.Workspaces...)
	s.state.Workspaces[index] = record
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return Workspace{}, err
	}
	return record, nil
}

func (s *workspaceStore) rename(path, name string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findPathLocked(path)
	if index < 0 {
		return Workspace{}, errors.New("workspace not found")
	}
	previous := cloneWorkspaceState(s.state)
	record := s.state.Workspaces[index]
	record.Name = name
	record.UpdatedAt = time.Now().UnixMilli()
	s.state.Workspaces = append([]Workspace(nil), s.state.Workspaces...)
	s.state.Workspaces[index] = record
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return Workspace{}, err
	}
	return record, nil
}

// remove 删除注册项；调用方负责先确认该 workspace 没有绑定会话。
// remove 删除注册项；调用方负责先确认该 workspace 没有绑定会话。
func (s *workspaceStore) remove(path string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findPathLocked(path)
	if index < 0 {
		return false, s.state.Selected, nil
	}
	previous := cloneWorkspaceState(s.state)
	removed := s.state.Workspaces[index]
	s.state.Workspaces = append(s.state.Workspaces[:index], s.state.Workspaces[index+1:]...)
	if s.state.Selected == removed.Path {
		s.state.Selected = ""
		if len(s.state.Workspaces) > 0 {
			best := 0
			for i := 1; i < len(s.state.Workspaces); i++ {
				if s.state.Workspaces[i].UpdatedAt > s.state.Workspaces[best].UpdatedAt {
					best = i
				}
			}
			s.state.Selected = s.state.Workspaces[best].Path
		}
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return false, s.state.Selected, err
	}
	return true, s.state.Selected, nil
}

func cloneWorkspaceState(state workspaceRegistryState) workspaceRegistryState {
	state.Workspaces = append([]Workspace(nil), state.Workspaces...)
	return state
}

// canonicalWorkspacePath 要求绝对路径、存在且是目录，并解析符号链接。
func canonicalWorkspacePath(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", errors.New("path is required")
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("path must be a directory")
	}
	return filepath.Clean(resolved), nil
}

func defaultWorkspaceName(path string) string {
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(filepath.Separator) {
		return path
	}
	return name
}

func workspacePathKey(path string) string {
	cleaned := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

// workspacePathKeyOf 用于匹配可能未规范化的旧会话路径。
func workspacePathKeyOf(path string) string {
	value := strings.TrimSpace(path)
	if value == "" {
		return ""
	}
	if absolute, err := filepath.Abs(value); err == nil {
		value = absolute
	}
	return workspacePathKey(value)
}
