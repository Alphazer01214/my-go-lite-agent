package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
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
	registryMu    sync.RWMutex
	registry      *Registry
	backend       CallBackend
	pluginStates  PluginStateProvider
	lifecycle     PluginLifecycle
	pickDirectory directoryPicker
	mu            sync.Mutex
	connections   map[*uiConnection]struct{}
	logger        func(string, ...any)
	diagnostics   []plugin.Diagnostic
	sequence      atomic.Uint64
	connectionSeq atomic.Uint64
}

// NewServer creates the WebUI transport around one Host call backend.
// cmd/liteagent-server owns the HTTP server lifecycle.
func NewServer(registry *Registry, backend CallBackend) *Server {
	if registry == nil {
		registry, _ = NewRegistry(plugin.Discoveries{})
	}
	bootstrap := registry.Bootstrap()
	return &Server{
		registry:      registry,
		backend:       backend,
		pickDirectory: pickDirectoryNative,
		connections:   make(map[*uiConnection]struct{}),
		diagnostics:   append([]plugin.Diagnostic(nil), bootstrap.Diagnostics...),
	}
}

// SetLogFunc sets the optional connection/diagnostic logger.
func (s *Server) SetLogFunc(fn func(string, ...any)) {
	s.mu.Lock()
	s.logger = fn
	s.mu.Unlock()
}

func (s *Server) log(format string, args ...any) {
	s.mu.Lock()
	fn := s.logger
	s.mu.Unlock()
	if fn != nil {
		fn(format, args...)
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

// currentRegistry borrows the active Registry. A Registry is immutable after
// NewRegistry, so callers may keep using the pointer after the lock is dropped.
func (s *Server) currentRegistry() *Registry {
	s.registryMu.RLock()
	defer s.registryMu.RUnlock()
	return s.registry
}

// replaceRegistry publishes a freshly built Registry. Readers that already hold
// the previous pointer finish against it; new readers see the new one.
func (s *Server) replaceRegistry(discoveries plugin.Discoveries) []plugin.Diagnostic {
	next, diagnostics := NewRegistry(discoveries)
	s.registryMu.Lock()
	s.registry = next
	s.registryMu.Unlock()
	return diagnostics
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
	mux.HandleFunc("POST /bridge/plugins/rescan", sameOriginHandler(s.servePluginRescan))
	mux.HandleFunc("POST /bridge/plugins/{plugin}/start", sameOriginHandler(s.servePluginStart))
	mux.HandleFunc("POST /bridge/plugins/{plugin}/stop", sameOriginHandler(s.servePluginStop))
	mux.HandleFunc("GET /bridge/ui", s.serveUI)
	mux.HandleFunc("POST /bridge/pick-directory", sameOriginHandler(s.servePickDirectory))
	mux.HandleFunc("POST /bridge/call", sameOriginHandler(func(w http.ResponseWriter, r *http.Request) {
		serveBrowserCall(w, r, s.backend)
	}))
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

// sameOriginHandler guards POST endpoints that mutate Host state, open local
// UI, or execute Capability Calls. Requests without an Origin header (curl,
// tests) pass; cross-origin browser posts are rejected.
func sameOriginHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request is not allowed", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// sameOrigin accepts requests that carry no browser-supplied origin marker
// (curl, tests) and rejects cross-origin browser posts.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
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
