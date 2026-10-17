package webui

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// PluginLifecycle is the Host management seam used by the unified plugin menu.
type PluginLifecycle interface {
	PluginSnapshot() []host.PluginSnapshot
	RescanPlugins() (plugin.Discoveries, error)
	StartPlugin(name string) error
	StopPlugin(name string) error
}

func (s *Server) servePluginRescan(w http.ResponseWriter, _ *http.Request) {
	manager := s.manager()
	if manager == nil {
		writeManagementError(w, http.StatusServiceUnavailable, "plugin_manager_unavailable", "plugin manager unavailable")
		return
	}
	discoveries, err := manager.RescanPlugins()
	if err != nil {
		writeManagementError(w, http.StatusBadRequest, "plugin_rescan_failed", err.Error())
		return
	}
	diagnostics := s.replaceRegistry(discoveries)
	s.mu.Lock()
	s.diagnostics = append([]plugin.Diagnostic(nil), diagnostics...)
	s.mu.Unlock()
	writeManagementBootstrap(w, s.Bootstrap())
}

func (s *Server) servePluginStart(w http.ResponseWriter, r *http.Request) {
	s.servePluginAction(w, r, "start")
}

func (s *Server) servePluginStop(w http.ResponseWriter, r *http.Request) {
	s.servePluginAction(w, r, "stop")
}

func (s *Server) servePluginAction(w http.ResponseWriter, r *http.Request, action string) {
	manager := s.manager()
	if manager == nil {
		writeManagementError(w, http.StatusServiceUnavailable, "plugin_manager_unavailable", "plugin manager unavailable")
		return
	}
	name := r.PathValue("plugin")
	if name == "" {
		writeManagementError(w, http.StatusBadRequest, "bad_arguments", "plugin name is required")
		return
	}

	var err error
	switch action {
	case "start":
		err = manager.StartPlugin(name)
	case "stop":
		err = manager.StopPlugin(name)
	default:
		err = errors.New("unknown plugin action")
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, host.ErrPluginNotMounted) {
			status = http.StatusNotFound
		}
		writeManagementError(w, status, "plugin_action_failed", err.Error())
		return
	}
	writeManagementBootstrap(w, s.Bootstrap())
}

func (s *Server) manager() PluginLifecycle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lifecycle
}

func writeManagementBootstrap(w http.ResponseWriter, bootstrap Bootstrap) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(bootstrap)
}

func writeManagementError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
