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
	"sync/atomic"
	"time"
)

const workspaceRegistryVersion = 1

type workspaceRecord struct {
	WorkspaceID      string `json:"workspace_id"`
	Name             string `json:"name"`
	Path             string `json:"path"`
	CreatedTimestamp int64  `json:"created_ts"`
	UpdatedTimestamp int64  `json:"updated_ts"`
}

type workspaceRegistryState struct {
	Version    int               `json:"version"`
	SelectedID string            `json:"selected_id,omitempty"`
	Workspaces []workspaceRecord `json:"workspaces"`
}

type workspaceStore struct {
	mu    sync.Mutex
	path  string
	state workspaceRegistryState
	seq   atomic.Int64
}

func workspaceDataFile(sessionDir string) string {
	if value := os.Getenv("SESSION_WORKSPACES_FILE"); value != "" {
		return value
	}
	return filepath.Join(sessionDir, "workspaces.json")
}

// legacyWorkspaceDataFile locates the pre-registry workspace file. The old
// layout kept it at <data-root>/workspace/workspaces.json while sessions lived
// in <data-root>/session/sessions, so the path is derived by walking two levels
// up from the session dir. A session dir outside that layout simply finds no
// legacy file; LEGACY_WORKSPACE_DATA_FILE overrides the guess.
func legacyWorkspaceDataFile(sessionDir string) string {
	if value := os.Getenv("LEGACY_WORKSPACE_DATA_FILE"); value != "" {
		return value
	}
	return filepath.Join(filepath.Dir(filepath.Dir(sessionDir)), "workspace", "workspaces.json")
}

func newWorkspaceStore(path, legacyPath string) (*workspaceStore, error) {
	store := &workspaceStore{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if legacyPath != "" {
			if legacy, legacyErr := os.ReadFile(legacyPath); legacyErr == nil {
				if decodeErr := json.Unmarshal(legacy, &store.state); decodeErr == nil {
					if store.state.Version == 0 {
						store.state.Version = workspaceRegistryVersion
					}
					if store.state.Version != workspaceRegistryVersion {
						return nil, fmt.Errorf("unsupported workspace registry version %d", store.state.Version)
					}
					if store.state.Workspaces == nil {
						store.state.Workspaces = []workspaceRecord{}
					}
					// The migration must not silently drop the registry: if the
					// copy fails, keep the loaded records in memory and say so.
					if saveErr := store.saveLocked(); saveErr != nil {
						return nil, fmt.Errorf("migrate workspace registry: %w", saveErr)
					}
					return store, nil
				}
			}
		}
		store.state = workspaceRegistryState{
			Version: workspaceRegistryVersion, Workspaces: []workspaceRecord{},
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
		store.state.Workspaces = []workspaceRecord{}
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

func (s *workspaceStore) findIDLocked(workspaceID string) int {
	for index, record := range s.state.Workspaces {
		if record.WorkspaceID == workspaceID {
			return index
		}
	}
	return -1
}

func (s *workspaceStore) findPathLocked(path string) int {
	key := workspacePathKey(path)
	for index, record := range s.state.Workspaces {
		if workspacePathKey(record.Path) == key {
			return index
		}
	}
	return -1
}

func (s *workspaceStore) ensure(path, name string, selectWorkspace bool) (workspaceRecord, bool, error) {
	canonical, err := canonicalWorkspacePath(path)
	if err != nil {
		return workspaceRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if index := s.findPathLocked(canonical); index >= 0 {
		record := s.state.Workspaces[index]
		if selectWorkspace && s.state.SelectedID != record.WorkspaceID {
			previous := cloneWorkspaceState(s.state)
			s.state.SelectedID = record.WorkspaceID
			record.UpdatedTimestamp = nowMS()
			s.state.Workspaces = append([]workspaceRecord(nil), s.state.Workspaces...)
			s.state.Workspaces[index] = record
			if err := s.saveLocked(); err != nil {
				s.state = previous
				return workspaceRecord{}, false, err
			}
		}
		return record, false, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultWorkspaceName(canonical)
	}
	now := nowMS()
	record := workspaceRecord{
		WorkspaceID:      fmt.Sprintf("ws-%d-%d", time.Now().UnixNano(), s.seq.Add(1)),
		Name:             name,
		Path:             canonical,
		CreatedTimestamp: now,
		UpdatedTimestamp: now,
	}
	previous := cloneWorkspaceState(s.state)
	s.state.Workspaces = append(s.state.Workspaces, record)
	if selectWorkspace || s.state.SelectedID == "" {
		s.state.SelectedID = record.WorkspaceID
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return workspaceRecord{}, false, err
	}
	return record, true, nil
}

func (s *workspaceStore) list() ([]workspaceRecord, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]workspaceRecord(nil), s.state.Workspaces...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedTimestamp != items[j].UpdatedTimestamp {
			return items[i].UpdatedTimestamp > items[j].UpdatedTimestamp
		}
		return items[i].WorkspaceID > items[j].WorkspaceID
	})
	return items, s.state.SelectedID
}

func (s *workspaceStore) info(workspaceID string) (workspaceRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceID)
	if index < 0 {
		return workspaceRecord{}, false
	}
	return s.state.Workspaces[index], true
}

func (s *workspaceStore) current() (*workspaceRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(s.state.SelectedID)
	if index < 0 {
		return nil, false
	}
	record := s.state.Workspaces[index]
	return &record, true
}

func (s *workspaceStore) selectID(workspaceID string) (workspaceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceID)
	if index < 0 {
		return workspaceRecord{}, errors.New("workspace not found")
	}
	record := s.state.Workspaces[index]
	if s.state.SelectedID == workspaceID {
		return record, nil
	}
	previous := cloneWorkspaceState(s.state)
	record.UpdatedTimestamp = nowMS()
	s.state.SelectedID = workspaceID
	s.state.Workspaces = append([]workspaceRecord(nil), s.state.Workspaces...)
	s.state.Workspaces[index] = record
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return workspaceRecord{}, err
	}
	return record, nil
}

func (s *workspaceStore) rename(workspaceID, name string) (workspaceRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return workspaceRecord{}, errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceID)
	if index < 0 {
		return workspaceRecord{}, errors.New("workspace not found")
	}
	previous := cloneWorkspaceState(s.state)
	record := s.state.Workspaces[index]
	record.Name = name
	record.UpdatedTimestamp = nowMS()
	s.state.Workspaces = append([]workspaceRecord(nil), s.state.Workspaces...)
	s.state.Workspaces[index] = record
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return workspaceRecord{}, err
	}
	return record, nil
}

func (s *workspaceStore) remove(workspaceID string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.findIDLocked(workspaceID)
	if index < 0 {
		return false, s.state.SelectedID, nil
	}
	previous := cloneWorkspaceState(s.state)
	s.state.Workspaces = append(s.state.Workspaces[:index], s.state.Workspaces[index+1:]...)
	if s.state.SelectedID == workspaceID {
		s.state.SelectedID = ""
		if len(s.state.Workspaces) > 0 {
			s.state.SelectedID = s.state.Workspaces[0].WorkspaceID
		}
	}
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return false, s.state.SelectedID, err
	}
	return true, s.state.SelectedID, nil
}

func canonicalWorkspacePath(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" {
		return "", errors.New("path is required")
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("path must be absolute")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
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
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(path))
	}
	return filepath.Clean(path)
}

func cloneWorkspaceState(state workspaceRegistryState) workspaceRegistryState {
	state.Workspaces = append([]workspaceRecord(nil), state.Workspaces...)
	return state
}
