package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"os"
	urlpath "path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	registryMu   sync.RWMutex
	registry     *Registry
	backend      CallBackend
	pluginStates PluginStateProvider
	lifecycle    PluginLifecycle
	mu           sync.Mutex
	connection   *uiConnection
	diagnostics  []plugin.Diagnostic
	sequence     atomic.Uint64
}

// NewServer creates the WebUI transport around one Host call backend.
// cmd/liteagent-server owns the HTTP server lifecycle.
func NewServer(registry *Registry, backend CallBackend) *Server {
	if registry == nil {
		registry, _ = NewRegistry(plugin.Discoveries{})
	}
	bootstrap := registry.Bootstrap()
	return &Server{
		registry:    registry,
		backend:     backend,
		diagnostics: append([]plugin.Diagnostic(nil), bootstrap.Diagnostics...),
	}
}

func (s *Server) SetPluginStateProvider(provider PluginStateProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pluginStates = provider
}

func (s *Server) SetPluginManager(manager PluginLifecycle) {
	s.mu.Lock()
	s.lifecycle = manager
	s.pluginStates = manager
	s.mu.Unlock()
}

func (s *Server) currentRegistry() *Registry {
	s.registryMu.RLock()
	defer s.registryMu.RUnlock()
	return s.registry
}

func (s *Server) replaceRegistry(discoveries plugin.Discoveries) []plugin.Diagnostic {
	s.registryMu.Lock()
	defer s.registryMu.Unlock()
	return s.registry.Replace(discoveries)
}

func (s *Server) Handler() http.Handler {
	platformRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serveIndex)
	mux.HandleFunc("GET /platform/{path...}", func(w http.ResponseWriter, r *http.Request) {
		serveEmbeddedAsset(w, r, platformRoot, r.PathValue("path"))
	})
	mux.HandleFunc("GET /bridge/bootstrap", s.serveBootstrap)
	mux.HandleFunc("POST /bridge/plugins/rescan", s.servePluginRescan)
	mux.HandleFunc("POST /bridge/plugins/{plugin}/start", s.servePluginStart)
	mux.HandleFunc("POST /bridge/plugins/{plugin}/stop", s.servePluginStop)
	mux.HandleFunc("GET /bridge/ui", s.serveUI)
	mux.HandleFunc("POST /bridge/call", func(w http.ResponseWriter, r *http.Request) {
		serveBrowserCall(w, r, s.backend)
	})
	mux.HandleFunc("GET /plugins/{plugin}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		servePluginResource(w, r, s.currentRegistry(), r.PathValue("plugin"), r.PathValue("path"))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasParentSegment(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'",
		)
		w.Header().Set("Cache-Control", "no-cache")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) Bootstrap() Bootstrap {
	bootstrap := s.currentRegistry().Bootstrap()
	s.mu.Lock()
	bootstrap.Diagnostics = append([]plugin.Diagnostic(nil), s.diagnostics...)
	provider := s.pluginStates
	s.mu.Unlock()
	if provider != nil {
		bootstrap.Plugins = mergePluginSnapshots(bootstrap.Plugins, provider.PluginSnapshot())
	}
	return bootstrap
}

func (s *Server) serveBootstrap(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(s.Bootstrap()); err != nil {
		http.Error(w, "failed to encode bootstrap", http.StatusInternalServerError)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedAsset(w, r, staticFiles, "static/index.html")
}

func servePluginResource(
	w http.ResponseWriter,
	r *http.Request,
	registry *Registry,
	pluginName, packagePath string,
) {
	realPath, ok := registry.ResolveResource(pluginName, urlpath.Clean(packagePath))
	if !ok {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(realPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	setContentType(w, urlpath.Base(packagePath))
	http.ServeContent(w, r, urlpath.Base(packagePath), info.ModTime(), file)
}

func serveEmbeddedAsset(w http.ResponseWriter, r *http.Request, root fs.FS, name string) {
	cleaned := urlpath.Clean("/" + strings.ReplaceAll(name, "\\", "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		http.NotFound(w, r)
		return
	}

	data, err := fs.ReadFile(root, cleaned)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	setContentType(w, urlpath.Base(cleaned))
	http.ServeContent(w, r, urlpath.Base(cleaned), time.Time{}, bytes.NewReader(data))
}

func setContentType(w http.ResponseWriter, name string) {
	extension := strings.ToLower(urlpath.Ext(name))
	switch extension {
	case ".js", ".mjs":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	default:
		if contentType := mime.TypeByExtension(extension); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
	}
}

func hasParentSegment(urlPath string) bool {
	for _, segment := range strings.Split(strings.ReplaceAll(urlPath, "\\", "/"), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}
