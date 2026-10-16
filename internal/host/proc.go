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
// plugin name as instance id, preserving the existing single-instance contract.
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
	if err := cmd.Start(); err != nil {
		return err
	}
	// 收尸：进程退出后 Wait 回收，避免僵尸；done 供 Close 等待退出
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	h.mu.Lock()
	if _, exists := h.plugins[instanceID]; exists {
		h.mu.Unlock()
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return fmt.Errorf("instance %s already exists: %w", instanceID, ErrInstanceExists)
	}
	h.gen[instanceID]++
	gen := h.gen[instanceID]
	h.plugins[instanceID] = &proc{
		discovery:      discovery,
		instanceID:     instanceID,
		pluginName:     manifest.Name,
		configRevision: configRevision,
		cmd:            cmd,
		stdin:          stdin,
		gen:            gen,
		healthy:        true,
		done:           done,
	}
	h.mu.Unlock()

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
	if p == nil || p.gen != gen {
		h.mu.Unlock()
		return
	}
	p.healthy = false
	h.hlog(fmt.Sprintf("instance unhealthy instance=%s plugin=%s generation=%d", instanceID, p.pluginName, gen))
	// 死者为被调方的 pending 需合成 plugin_down；仅为调用方的 pending 直接丢弃
	//（调用方进程已亡，其 SDK pending 随进程消亡）。
	var dead []*wait
	for id, wt := range h.pending {
		if wt.target != instanceID && wt.caller != instanceID {
			continue
		}
		delete(h.pending, id)
		if wt.target == instanceID {
			dead = append(dead, wt)
		}
	}
	h.mu.Unlock()

	for _, wt := range dead {
		if wt.kind == WaitHost && wt.resCh != nil {
			select {
			case wt.resCh <- &Frame{
				Version:    FrameVersion,
				ID:         wt.frameID,
				Type:       FrameResponse,
				Capability: wt.capability,
				Method:     wt.method,
				From:       FromHost,
				ErrorCode:  CodePluginDown,
				ErrorMsg:   fmt.Sprintf("plugin instance %s is down", instanceID),
			}:
			default:
			}
			continue
		}
		h.replyError(wt.caller, wt.frameID, wt.capability, wt.method,
			CodePluginDown, fmt.Sprintf("plugin instance %s is down", instanceID))
	}
}
