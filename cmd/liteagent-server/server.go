package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"time"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/internal/webui"
	"github.com/tomori/my-go-lite-agent/protocol"
)

const defaultHTTPAddress = "127.0.0.1:2333"

// run 启动 Host 和 WebUI 并保持运行。纯 WebUI 方案：无命令行交互，仅日志展示到 stderr。
// 退出码：0 正常；1 启动或 HTTP 服务失败。
func run() int {
	fmt.Fprintf(os.Stderr, "liteagent-server %s\n", version)
	pluginDir := pluginsDir()
	discoveries := plugin.ScanRoot(pluginDir)
	registry, diagnostics := webui.NewRegistry(discoveries)
	for _, diagnostic := range diagnostics {
		fmt.Fprintf(os.Stderr, "scan: %s\n", diagnostic)
	}

	listener, err := net.Listen("tcp", httpAddress())
	if err != nil {
		fmt.Fprintf(os.Stderr, "webui listen: %v\n", err)
		return 1
	}
	defer listener.Close()

	h, err := host.Run(discoveries, nil) // nil → Host 日志走 stderr
	if err != nil {
		fmt.Fprintf(os.Stderr, "host run: %v\n", err)
		return 1
	}
	defer h.Close()
	h.SetPluginDir(pluginDir)
	h.SetTraceFunc(traceTo(os.Stderr))

	mounted := h.GetMountedPlugins()
	sort.Strings(mounted)
	fmt.Fprintf(os.Stderr, "mounted: %v\n", mounted)

	uiServer := webui.NewServer(registry, h)
	uiServer.SetPluginManager(h)
	h.SetUIPublisher(uiServer)

	server := &http.Server{
		Handler:           uiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	fmt.Fprintf(os.Stderr, "webui: %s\n", serverURL(listener.Addr().String()))

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	select {
	case <-signals:
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "webui serve: %v\n", err)
			return 1
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "webui shutdown: %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "\nshutting down…")
	return 0
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

// traceTo 打印 Host 边界上的每条 Frame（log 展示）。
func traceTo(w *os.File) host.TraceFunc {
	return func(dir, name string, f *protocol.Frame) {
		payload := f.Payload
		if len(payload) > 200 {
			payload = append(append([]byte(nil), payload[:200]...), []byte("…")...)
		}
		fmt.Fprintf(w, "[%s] %s id=%s type=%s cap=%s method=%s from=%s err=%s payload=%s\n",
			dir, name, f.ID, f.Type, f.Capability, f.Method, f.From, f.ErrorCode, payload)
	}
}
