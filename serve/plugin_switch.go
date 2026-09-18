package serve

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// SwitchFileName is the Host-owned plugin enable/disable store inside pluginsDir.
// L0 data only: a list of plugin names. ADR-0032.
const SwitchFileName = ".plugin-switch.json"

type pluginSwitchFile struct {
	Disabled []string `json:"disabled"`
}

// SwitchPath returns the switch file path for a Discovery root.
func SwitchPath(pluginsDir string) string {
	dir := pluginsDir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, SwitchFileName)
}

// LoadPluginSwitchFile reads the disabled name set; missing/invalid → empty.
func LoadPluginSwitchFile(path string) map[string]bool {
	out := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var f pluginSwitchFile
	if err := json.Unmarshal(raw, &f); err != nil {
		fmt.Fprintf(os.Stderr, "plugin switch: bad %s: %v\n", path, err)
		return out
	}
	for _, n := range f.Disabled {
		if n != "" {
			out[n] = true
		}
	}
	return out
}

func savePluginSwitchFile(path string, disabled map[string]bool) error {
	if path == "" {
		return fmt.Errorf("plugin switch path not set")
	}
	var f pluginSwitchFile
	for n := range disabled {
		if disabled[n] {
			f.Disabled = append(f.Disabled, n)
		}
	}
	sort.Strings(f.Disabled)
	if f.Disabled == nil {
		f.Disabled = []string{}
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, raw, 0o600)
}

// SetDisabledSet replaces the in-memory disabled set (boot path).
func (s *Server) SetDisabledSet(disabled map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled == nil {
		s.disabled = map[string]bool{}
	}
	for k := range s.disabled {
		delete(s.disabled, k)
	}
	for k, v := range disabled {
		if v {
			s.disabled[k] = true
		}
	}
}

// LoadPluginSwitch reads SwitchPath(pluginsDir) into memory and records the path.
func (s *Server) LoadPluginSwitch() {
	s.mu.Lock()
	path := SwitchPath(s.pluginsDir)
	s.switchPath = path
	if s.disabled == nil {
		s.disabled = map[string]bool{}
	}
	s.mu.Unlock()
	set := LoadPluginSwitchFile(path)
	s.SetDisabledSet(set)
}

// IsPluginDisabled reports whether name is in the user switch-off set.
func (s *Server) IsPluginDisabled(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.disabled[name]
}

// DisabledPluginNames returns sorted disabled plugin names.
func (s *Server) DisabledPluginNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return disabledNamesLocked(s.disabled)
}

func disabledNamesLocked(m map[string]bool) []string {
	var out []string
	for n, v := range m {
		if v {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

// FilterMountedFound drops disabled plugins from a Discovery mount list (ADR-0032).
func FilterMountedFound(mounted []discovery.Found, disabled map[string]bool) []discovery.Found {
	if len(disabled) == 0 {
		return mounted
	}
	out := make([]discovery.Found, 0, len(mounted))
	for _, p := range mounted {
		name := p.Manifest.Name
		if disabled[name] {
			fmt.Fprintf(os.Stderr, "plugin switch: %s disabled, skipping mount\n", name)
			continue
		}
		out = append(out, p)
	}
	return out
}

// SetPluginEnabled toggles a plugin by name (ADR-0032). Disable unmounts
// immediately and blocks re-launch; enable only clears the denylist.
func (s *Server) SetPluginEnabled(name string, enabled bool) (map[string]any, error) {
	if name == "" {
		return nil, fmt.Errorf("plugin name required")
	}
	s.mu.Lock()
	if s.disabled == nil {
		s.disabled = map[string]bool{}
	}
	if s.switchPath == "" {
		s.switchPath = SwitchPath(s.pluginsDir)
	}
	if enabled {
		delete(s.disabled, name)
	} else {
		s.disabled[name] = true
	}
	snapshot := map[string]bool{}
	for k, v := range s.disabled {
		if v {
			snapshot[k] = true
		}
	}
	path := s.switchPath
	s.mu.Unlock()

	if err := savePluginSwitchFile(path, snapshot); err != nil {
		return nil, err
	}
	if !enabled {
		s.unmountPlugin(name)
	}
	return map[string]any{
		"ok":       true,
		"name":     name,
		"enabled":  enabled,
		"disabled": disabledNamesLocked(snapshot),
	}, nil
}

// unmountPlugin kills the process/UI entry for name and fails pending calls.
func (s *Server) unmountPlugin(name string) {
	s.mu.Lock()
	p := s.plugins[name]
	delete(s.plugins, name)
	delete(s.mountedUI, name)
	var fails []*wait
	var pluginFail []struct{ caller, origID string }
	for id, w := range s.pending {
		if w.target == name {
			delete(s.pending, id)
			if w.kind == waitHost {
				fails = append(fails, w)
			} else {
				pluginFail = append(pluginFail, struct{ caller, origID string }{w.caller, w.origID})
			}
		}
	}
	s.mu.Unlock()

	if p != nil {
		if p.stdin != nil {
			p.wmu.Lock()
			_ = p.stdin.Close()
			p.wmu.Unlock()
		}
		if p.cmd != nil {
			killTree(p.cmd)
		}
	}
	for _, w := range fails {
		select {
		case w.ch <- &CallResult{Frame: &protocol.Frame{
			Type:  protocol.TypeRes,
			Error: &protocol.FrameError{Code: "plugin_down", Message: name + " disabled"},
		}}:
		default:
		}
	}
	for _, pf := range pluginFail {
		_ = s.writeTo(pf.caller, &protocol.Frame{
			Type: protocol.TypeRes,
			ID:   pf.origID,
			Error: &protocol.FrameError{Code: "plugin_down", Message: name + " disabled"},
		})
	}
	s.reconcileConsumes()
}

// hostPluginsSnapshot is the JSON body for host.plugins / CallHost.
type hostPluginsSnapshot struct {
	Plugins  []hostPluginItem `json:"plugins"`
	Disabled []string         `json:"disabled"`
}

type hostPluginItem struct {
	Name     string   `json:"name"`
	Provides []string `json:"provides,omitempty"`
	Healthy  bool     `json:"healthy"`
	UI       bool     `json:"ui,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
}

func (s *Server) hostPluginsSnapshot() hostPluginsSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := hostPluginsSnapshot{Plugins: []hostPluginItem{}, Disabled: disabledNamesLocked(s.disabled)}
	for name, p := range s.plugins {
		item := hostPluginItem{Name: name, Healthy: p != nil && p.healthy, Disabled: s.disabled[name]}
		if p != nil {
			item.Provides = append([]string(nil), p.found.Manifest.Provides...)
		}
		out.Plugins = append(out.Plugins, item)
	}
	for name := range s.mountedUI {
		out.Plugins = append(out.Plugins, hostPluginItem{Name: name, UI: true, Healthy: true, Disabled: s.disabled[name]})
	}
	return out
}
