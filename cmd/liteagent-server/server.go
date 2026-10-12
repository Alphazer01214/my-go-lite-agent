package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"

	"github.com/tomori/my-go-lite-agent/internal/host"
	"github.com/tomori/my-go-lite-agent/internal/plugin"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// run 启动宿主并保持运行。纯 WebUI 方案：无命令行交互，仅日志展示到 stderr。
// 退出码：0 正常；1 启动失败。
func run() int {
	discs := plugin.ScanRoot(pluginsDir())
	for _, e := range discs.Errors {
		fmt.Fprintf(os.Stderr, "scan: %v\n", e)
	}

	h, err := host.Run(discs, nil) // nil → Host 日志走 stderr
	if err != nil {
		fmt.Fprintf(os.Stderr, "host run: %v\n", err)
		return 1
	}
	defer h.Close()
	h.SetTraceFunc(traceTo(os.Stderr))

	mounted := h.GetMountedPlugins()
	sort.Strings(mounted)
	fmt.Fprintf(os.Stderr, "mounted: %v\n", mounted)

	// 保持宿主存活直到退出信号（WebUI 接管后此处改为启动 HTTP 服务）
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	fmt.Fprintln(os.Stderr, "\nshutting down…")
	return 0
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
