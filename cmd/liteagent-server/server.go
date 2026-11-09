package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/logging"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/internal/webui"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const defaultHTTPAddress = "127.0.0.1:2333"

// run 启动 Host 和 WebUI 并保持运行。纯 WebUI 方案：无命令行交互，日志默认展示到 stderr。
// 退出码：0 正常；1 启动或 HTTP 服务失败。
func run() int {
	logOut, closeLog := setupLogOutput()
	defer closeLog()
	logf := func(level, format string, args ...any) {
		logLine(logOut, level, format, args...)
	}

	logf("INFO", "liteagent-server %s", version)
	pluginDir := pluginsDir()
	discoveries := plugin.ScanRoot(pluginDir)
	registry, diagnostics := webui.NewRegistry(discoveries)
	for _, diagnostic := range diagnostics {
		logf("WARN", "scan: %s", diagnostic)
	}

	listener, err := net.Listen("tcp", httpAddress())
	if err != nil {
		logf("ERROR", "webui listen: %v", err)
		return 1
	}
	defer listener.Close()

	// Build the Host first and start it only after the UI publisher is wired in:
	// plugins publish ui.state as soon as they boot, and an unmounted publisher
	// would answer ui_not_mounted.
	h := host.New(discoveries, func(message string) {
		logf("INFO", "%s", message)
	})
	defer h.Close()
	h.SetPluginDir(pluginDir)
	h.SetTraceFunc(traceTo(logOut, traceMode()))

	uiServer := webui.NewServer(registry, h)
	uiServer.SetLogFunc(func(format string, args ...any) {
		logf("INFO", format, args...)
	})
	uiServer.SetPluginManager(h)
	h.SetUIPublisher(uiServer)

	h.Start()

	mounted := h.GetMountedPlugins()
	sort.Strings(mounted)
	logf("INFO", "mounted: %v", mounted)

	server := &http.Server{
		Handler:           uiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	logf("INFO", "webui: %s", serverURL(listener.Addr().String()))

	signals := make(chan os.Signal, 1)
	// SIGTERM matters on Linux/container stop: without it the process is killed
	// outright and defer h.Close() never runs, leaving plugin processes behind.
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("ERROR", "webui serve: %v", err)
			return 1
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logf("ERROR", "webui shutdown: %v", err)
		return 1
	}
	logf("INFO", "shutting down…")
	return 0
}

func setupLogOutput() (io.Writer, func()) {
	path := strings.TrimSpace(os.Getenv("LITEAGENT_LOG_FILE"))
	if path == "" {
		return os.Stderr, func() {}
	}
	maxBytes := int64(envInt("LITEAGENT_LOG_MAX_BYTES", 8<<20))
	writer, err := logging.New(path, maxBytes, envInt("LITEAGENT_LOG_BACKUPS", 3))
	if err != nil {
		fmt.Fprintf(os.Stderr, "liteagent-server: open log file %s: %v\n", path, err)
		return os.Stderr, func() {}
	}
	return writer, func() {
		_ = writer.Close()
	}
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func logLine(w io.Writer, level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(w, "%s %-5s %s\n",
		time.Now().Format("2006-01-02T15:04:05.000Z07:00"), level, message)
}

func traceMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LITEAGENT_TRACE"))) {
	case "off":
		return "off"
	case "full":
		return "full"
	default:
		return "summary"
	}
}

func httpAddress() string {
	if address := os.Getenv("LITEAGENT_HTTP_ADDR"); address != "" {
		return address
	}
	return defaultHTTPAddress
}

func serverURL(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "http://" + address + "/"
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

// pluginsDir 定位插件目录：优先取可执行文件旁的 plugins/（dist 布局），
// 否则用 CWD 的 plugins/。无 CLI flag，纯约定。
func pluginsDir() string {
	if exe, err := os.Executable(); err == nil {
		if side := filepath.Join(filepath.Dir(exe), "plugins"); isDir(side) {
			return side
		}
	}
	return "plugins"
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// traceTo 打印 Host 边界上的 Frame。summary 模式跳过高频 chunk 帧。
func traceTo(w io.Writer, mode string) host.TraceFunc {
	if mode == "off" {
		return nil
	}
	return func(dir, name string, f *protocol.Frame) {
		if mode == "summary" && f.Method == "chunk" {
			return
		}
		payload := f.Payload
		if len(payload) > 200 {
			// Cut on a rune boundary: a raw byte cut can split a multi-byte
			// character and emit invalid UTF-8 into the log stream.
			truncated := payload[:200]
			for len(truncated) > 0 && !utf8.Valid(truncated) {
				truncated = truncated[:len(truncated)-1]
			}
			payload = append(append([]byte(nil), truncated...), []byte("…")...)
		}
		logLine(w, "TRACE", "[%s] %s id=%s type=%s cap=%s method=%s from=%s err=%s payload=%s",
			dir, name, f.ID, f.Type, f.Capability, f.Method, f.From, f.ErrorCode, payload)
	}
}
