package main

import (
	"crypto/rand"
	"encoding/hex"
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

const workspaceRegistryVersion = 1

// workspaceRegistryState 与旧版 workspaces.json 保持同一 schema，
// 这样 dist 里已有的注册表与 build.ps1 的迁移逻辑可以直接接上。
type workspaceRegistryState struct {
	Version    int         `json:"version"`
	SelectedId string      `json:"selected_id,omitempty"`
	Workspaces []Workspace `json:"workspaces"`
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
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, fmt.Errorf("decode workspace registry: %w", err)
	}
	if store.state.Version == 0 {
		store.state.Version = workspaceRegistryVersion
	}
	if store.state.Version != workspaceRegistryVersion {
		return nil, fmt.Errorf("unsupported workspace registry version %d", store.state.Version)
	}
	if store.state.Workspaces == nil {
		store.state.Workspaces = []Workspace{}
	}
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

func (s *workspaceStore) findIDLocked(workspaceId string) int {
	for i, record := range s.state.Workspaces {
		if record.WorkspaceId == workspaceId {
			return i
		}
	}
	return -1
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
		return items[i].WorkspaceId > items[j].WorkspaceId
	})
	return items, s.state.SelectedId
}

func (s *workspaceStore) info(workspaceId string) (Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceId)
	if index < 0 {
		return Workspace{}, false
	}
	return s.state.Workspaces[index], true
}

func (s *workspaceStore) current() (*Workspace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(s.state.SelectedId)
	if index < 0 {
		return nil, false
	}
	record := s.state.Workspaces[index]
	return &record, true
}

func (s *workspaceStore) selectedID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.SelectedId
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
		if selectWorkspace && s.state.SelectedId != record.WorkspaceId {
			previous := cloneWorkspaceState(s.state)
			record.UpdatedAt = time.Now().UnixMilli()
			s.state.SelectedId = record.WorkspaceId
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
	id, err := s.newIDLocked()
	if err != nil {
		return Workspace{}, false, err
	}
	now := time.Now().UnixMilli()
	record := Workspace{
		WorkspaceId: id,
		Name:        name,
		Path:        canonical,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	previous := cloneWorkspaceState(s.state)
	s.state.Workspaces = append(s.state.Workspaces, record)
	if selectWorkspace || s.state.SelectedId == "" {
		s.state.SelectedId = record.WorkspaceId
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return Workspace{}, false, err
	}
	return record, true, nil
}

func (s *workspaceStore) newIDLocked() (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		id := fmt.Sprintf("ws-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
		if s.findIDLocked(id) < 0 {
			return id, nil
		}
	}
	return "", errors.New("could not allocate a unique workspace id")
}

func (s *workspaceStore) selectID(workspaceId string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceId)
	if index < 0 {
		return Workspace{}, errors.New("workspace not found")
	}
	record := s.state.Workspaces[index]
	if s.state.SelectedId == workspaceId {
		return record, nil
	}
	previous := cloneWorkspaceState(s.state)
	record.UpdatedAt = time.Now().UnixMilli()
	s.state.SelectedId = workspaceId
	s.state.Workspaces = append([]Workspace(nil), s.state.Workspaces...)
	s.state.Workspaces[index] = record
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return Workspace{}, err
	}
	return record, nil
}

func (s *workspaceStore) rename(workspaceId, name string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Workspace{}, errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceId)
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
func (s *workspaceStore) remove(workspaceId string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceId)
	if index < 0 {
		return false, s.state.SelectedId, nil
	}
	previous := cloneWorkspaceState(s.state)
	s.state.Workspaces = append(s.state.Workspaces[:index], s.state.Workspaces[index+1:]...)
	if s.state.SelectedId == workspaceId {
		s.state.SelectedId = ""
		if len(s.state.Workspaces) > 0 {
			best := 0
			for i := 1; i < len(s.state.Workspaces); i++ {
				if s.state.Workspaces[i].UpdatedAt > s.state.Workspaces[best].UpdatedAt {
					best = i
				}
			}
			s.state.SelectedId = s.state.Workspaces[best].WorkspaceId
		}
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return false, s.state.SelectedId, err
	}
	return true, s.state.SelectedId, nil
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
