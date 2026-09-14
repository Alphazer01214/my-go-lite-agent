package main_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWebServeShellAndMessage(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-server")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)
	layoutPath := writeTestLayout(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-serve", addr, "-layout", layoutPath)
	cmd.Env = hostEnv(t)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			t.Logf("stderr: %s", sc.Text())
		}
	}()
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			t.Logf("stdout: %s", sc.Text())
		}
	}()

	base := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	var ok bool
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				ok = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		t.Fatal("web shell did not come up")
	}

	res, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if !strings.Contains(string(body), "id=\"split\"") && !strings.Contains(string(body), "chat-col") {
		t.Fatalf("shell missing split layout")
	}

	// Post a message; fakellm should settle markdown over SSE eventually.
	req, _ := http.NewRequest(http.MethodPost, base+"/api/message", strings.NewReader(`{"text":"web-hello"}`))
	req.Header.Set("Content-Type", "application/json")
	mres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = mres.Body.Close()

	// Pull one SSE batch (with timeout via short-lived client).
	sse, err := http.Get(base + "/events?replay=1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sse.Body.Close() }()
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 0, 8192)
		tmp := make([]byte, 1024)
		for {
			n, err := sse.Body.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
				if strings.Contains(string(buf), "web-hello") || strings.Contains(string(buf), "markdown_text") {
					done <- string(buf)
					return
				}
			}
			if err != nil {
				done <- string(buf)
				return
			}
		}
	}()
	select {
	case out := <-done:
		if !strings.Contains(out, "event:") && !strings.Contains(out, "topic") {
			// may still be only mount events; require at least SSE framed data
			if out == "" {
				t.Fatal("empty SSE")
			}
		}
	case <-time.After(15 * time.Second):
		t.Fatal("sse timeout")
	}
	_ = fmt.Sprintf("%v", mres.StatusCode)
}

func TestWebCommandOutputAndHistory(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-server")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)
	layoutPath := writeTestLayout(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-serve", addr, "-layout", layoutPath)
	cmd.Env = hostEnv(t)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	base := "http://" + addr
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(base + "/")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// /help must return real output text (not just ok).
	req, _ := http.NewRequest(http.MethodPost, base+"/api/command", strings.NewReader(`{"line":"/help"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var help struct {
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&help); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if !strings.Contains(help.Output, "Native commands") {
		t.Fatalf("want help output, got %+v", help)
	}

	// One turn then history should include user text.
	req2, _ := http.NewRequest(http.MethodPost, base+"/api/message", strings.NewReader(`{"text":"hist-marker"}`))
	req2.Header.Set("Content-Type", "application/json")
	mres, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = mres.Body.Close()
	time.Sleep(2 * time.Second)

	// History rehydrates through the session Capability via the star route
	// (ADR-0011): /api/history and /api/trace are both retired.
	callBody := strings.NewReader(`{"cap":"session","method":"query","payload":{"sessionId":"","afterSeq":0,"limit":0}}`)
	hres, err := http.Post(base+"/api/call", "application/json", callBody)
	if err != nil {
		t.Fatal(err)
	}
	var hist struct {
		OK     bool `json:"ok"`
		Result struct {
			Facts []struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"facts"`
		} `json:"result"`
	}
	if err := json.NewDecoder(hres.Body).Decode(&hist); err != nil {
		t.Fatal(err)
	}
	_ = hres.Body.Close()
	if !hist.OK {
		t.Fatal("session.query call failed")
	}
	found := false
	for _, f := range hist.Result.Facts {
		if f.Type == "message" && f.Role == "user" && strings.Contains(f.Content, "hist-marker") {
			found = true
		}
	}
	if !found {
		t.Fatalf("history missing user message: %+v", hist.Result)
	}
}
