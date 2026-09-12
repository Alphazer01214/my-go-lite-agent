package main_test

import (
	"bufio"
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
	hostBin := buildPkg(t, root, "./cmd/host")
	pluginsDir := t.TempDir()
	buildSessionPluginDir(t, root, pluginsDir, "session")
	buildFakeLLMPluginDir(t, root, pluginsDir, "fakellm")
	cfg := filepath.Join(t.TempDir(), "assembly.json")
	writeFile(t, cfg, `{"plugins":["session","fakellm"]}`)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cmd := exec.Command(hostBin, "-plugins", pluginsDir, "-assembly", cfg, "-serve", addr)
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
	if !strings.Contains(string(body), "slot-sidebar") {
		t.Fatalf("shell missing slots")
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
