package host

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tomori/my-go-lite-agent/internal/plugin"
)

// launch the plugin by add a proc.
// entry 相对 discovery.Dir 解析；无扩展名时补试 .exe（源码 manifest 与构建产物命名兼容）。
// cmd.Dir 必须是插件目录：插件按 CWD 读 config.json，session 写 ./sessions。
func (h *Host) launch(discovery plugin.Discovery) error {
	return h.launchInstance(discovery.Manifest.Name, discovery, "")
}

// launchInstance starts one process activation. The default activation uses the
// plugin name as instance id, preserving the existing single-instance behavior.
func (h *Host) launchInstance(instanceID string, discovery plugin.Discovery, configRevision string) error {
	manifest := discovery.Manifest
	if instanceID == "" {
		return fmt.Errorf("instance id is required: %w", ErrBadArguments)
	}
	if h.IsPluginDisabled(manifest.Name) {
		return ErrPluginDisabled
	}
	path, err := resolveEntry(discovery.Dir, manifest.Entry)
	if err != nil {
		return err
	}
	cmd := exec.Command(path)
	// cmd.Dir 必须是**绝对**路径：Windows 下子进程以 Dir 为 cwd，若 Path 为相对路径
	// 会被按子进程 cwd 再解析一次（找不到）。resolveEntry 返回绝对路径，Dir 也绝对化。
	if dir, err := filepath.Abs(discovery.Dir); err == nil {
		cmd.Dir = dir
	} else {
		cmd.Dir = discovery.Dir
	}
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	h.mu.Lock()
	if _, exists := h.plugins[instanceID]; exists {
		h.mu.Unlock()
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("instance %s already exists: %w", instanceID, ErrInstanceExists)
	}
	h.gen[instanceID]++
	gen := h.gen[instanceID]
	done := make(chan struct{})
	p := &proc{
		discovery:      discovery,
		instanceID:     instanceID,
		pluginName:     manifest.Name,
		configRevision: configRevision,
		cmd:            cmd,
		stdin:          stdin,
		gen:            gen,
		state:          ProviderStateLoading,
		done:           done,
	}
	h.plugins[instanceID] = p
	h.mu.Unlock()

	if err := cmd.Start(); err != nil {
		message := fmt.Sprintf("plugin start failed: %v", err)
		h.mu.Lock()
		p.state = ProviderStateFailed
		p.failureCode = CodePluginDown
		p.failureMessage = message
		h.mu.Unlock()
		_ = stdin.Close()
		_ = stdout.Close()
		close(done)
		return err
	}

	// 收尸：进程退出后 Wait 回收，避免僵尸；done 供 Close 等待退出
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	go h.read(instanceID, manifest.Name, gen, stdout)
	return nil
}

// resolveEntry 把 manifest.Entry 解析为**绝对**可执行文件路径。
// 无扩展名时先试原名再试 .exe，源码清单（entry:"agent"）与 Windows 产物（agent.exe）都兼容。
// 必须返回绝对路径：cmd.Dir 非空时相对 Path 会被按子进程 cwd 解析而找不到。
func resolveEntry(dir, entry string) (string, error) {
	path := filepath.Join(dir, entry)
	if _, err := os.Stat(path); err != nil && filepath.Ext(entry) == "" {
		if _, err := os.Stat(path + ".exe"); err == nil {
			path += ".exe"
		}
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("plugin entry not found: %s", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve plugin entry %s: %w", path, err)
	}
	return abs, nil
}

// markUnhealthy 标记插件不健康并做死亡闭包：该插件相关的 pending 全部闭环，
// 防止调用方永久挂起（v1 无调用超时）。合成响应在锁外写，避免与 read/write 互锁。
func (h *Host) markUnhealthy(instanceID string, gen int) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	p := h.plugins[instanceID]
	if p == nil || p.gen != gen || p.state == ProviderStateInactive {
		h.mu.Unlock()
		return
	}
	if p.state == ProviderStateStopping {
		p.state = ProviderStateInactive
		h.mu.Unlock()
		return
	}
	message := fmt.Sprintf("plugin instance %s is down", instanceID)
	p.state = ProviderStateFailed
	p.failureCode = CodePluginDown
	p.failureMessage = message
	h.hlog(fmt.Sprintf("instance failed instance=%s plugin=%s generation=%d", instanceID, p.pluginName, gen))
	h.mu.Unlock()

	// 死者为被调方的 pending 合成 plugin_down；仅为调用方的 pending 直接丢弃。
	h.failInstancePending(instanceID, CodePluginDown, message)
}

func (h *Host) failProviderRegistration(instanceID, code, message string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.plugins[instanceID]
	if p == nil {
		return
	}
	p.state = ProviderStateFailed
	p.failureCode = code
	p.failureMessage = message
	p.registrations = nil
}
