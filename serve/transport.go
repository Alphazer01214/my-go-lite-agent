package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/protocol"
)

type waitKind int

const (
	waitHost waitKind = iota
	waitPlugin
)

type wait struct {
	kind       waitKind
	caller     string
	target     string
	origID     string
	cap        string
	method     string
	reqPayload json.RawMessage
	ch         chan *CallResult
	events     []*protocol.Frame
	onEvent    func(*protocol.Frame)
}

// CallResult carries one Call's final res plus any evt frames collected while waiting.
type CallResult struct {
	Frame  *protocol.Frame
	Events []*protocol.Frame
}

type proc struct {
	found   discovery.Found
	cmd     *exec.Cmd
	stdin   *os.File
	wmu     sync.Mutex
	healthy bool
	timeout time.Duration
	gen     int
}

// ensureCatalog returns a fresh Discovery result when pluginsDir is known.
func (s *Server) ensureCatalog() discovery.Result {
	s.mu.Lock()
	dir := s.pluginsDir
	cached := s.catalog
	s.mu.Unlock()
	if dir != "" {
		if res := discovery.Scan(dir); len(res.Plugins) > 0 {
			s.mu.Lock()
			s.catalog = res
			s.mu.Unlock()
			return res
		}
	}
	return cached
}

func (s *Server) launch(found discovery.Found) error {
	name := found.Manifest.Name
	entry := found.Manifest.ResolveEntry(found.Dir)
	cmd := exec.Command(entry)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin %s: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("stdout %s: %w", name, err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start %s: %w", name, err)
	}
	// Bind child to Host lifetime (Windows Job Object: kill-on-parent-close).
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()
	if err := job.assign(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "warn: assign %s to job: %v\n", name, err)
	}
	to := DefaultCallTimeout
	if found.Manifest.TimeoutMs > 0 {
		to = time.Duration(found.Manifest.TimeoutMs) * time.Millisecond
	}
	debugf("launch start plugin=%s", name)
	s.mu.Lock()
	s.gen[name]++
	g := s.gen[name]
	if old, ok := s.plugins[name]; ok && old != nil && old.stdin != nil {
		old.wmu.Lock()
		_ = old.stdin.Close()
		old.wmu.Unlock()
	}
	s.plugins[name] = &proc{
		found:   found,
		cmd:     cmd,
		stdin:   stdin.(*os.File),
		healthy: true,
		timeout: to,
		gen:     g,
	}
	s.mu.Unlock()
	debugf("launch plugin=%s gen=%d entry=%s", name, g, entry)
	go s.readLoop(name, g, stdout)
	return nil
}

func (s *Server) readLoop(pluginName string, gen int, stdout io.Reader) {
	for {
		f, err := protocol.ReadFrame(stdout)
		if err != nil {
			debugf("<- plugin=%s read_loop_exit err=%v", pluginName, err)
			s.markUnhealthy(pluginName, gen)
			return
		}
		debugFrame("<-", pluginName, f)
		s.handleFromPlugin(pluginName, f)
	}
}

func (s *Server) markUnhealthy(pluginName string, gen int) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	p := s.plugins[pluginName]
	if p == nil || p.gen != gen {
		s.mu.Unlock()
		return
	}
	p.healthy = false
	debugf("unhealthy plugin=%s gen=%d", pluginName, gen)
	var hostFail []*wait
	var pluginFail []struct{ caller, origID string }
	for id, w := range s.pending {
		if w.target == pluginName {
			delete(s.pending, id)
			if w.kind == waitHost {
				hostFail = append(hostFail, w)
			} else {
				pluginFail = append(pluginFail, struct{ caller, origID string }{w.caller, w.origID})
			}
		}
	}
	s.mu.Unlock()

	for _, w := range hostFail {
		select {
		case w.ch <- &CallResult{Frame: &protocol.Frame{
			Type:  protocol.TypeRes,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		}}:
		default:
		}
	}
	for _, pf := range pluginFail {
		_ = s.writeTo(pf.caller, &protocol.Frame{
			Type:  protocol.TypeRes,
			ID:    pf.origID,
			Error: &protocol.FrameError{Code: "plugin_down", Message: pluginName + " closed"},
		})
	}
	// A dead plugin can no longer serve its provides: re-register the registry
	// so its consumers degrade and routing fails honestly (ADR-0022).
	s.reconcileConsumes()
}

func (s *Server) ensureAlive(name string) error {
	s.mu.Lock()
	p := s.plugins[name]
	if p == nil {
		s.mu.Unlock()
		return fmt.Errorf("plugin %s not mounted", name)
	}
	if p.healthy {
		s.mu.Unlock()
		return nil
	}
	found := p.found
	old := p.cmd
	s.mu.Unlock()

	// Reap old process without hanging the caller. Do not Wait here:
	// Close owns Wait; a second Wait on the same Cmd panics.
	if old != nil {
		killTree(old)
	}
	s.mu.Lock()
	// Another goroutine may have restarted already.
	if cur := s.plugins[name]; cur != nil && cur.healthy {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	if err := s.launch(found); err != nil {
		return err
	}
	// The revived plugin may now satisfy consumes and its tools may differ
	// after the crash-restart: reconcile the registry and re-discover tools.
	go func() {
		s.reconcileConsumes()
		_ = s.discoverTools()
	}()
	return nil
}

func (s *Server) handleFromPlugin(from string, f *protocol.Frame) {
	switch f.Type {
	case protocol.TypeReq:
		s.routeRequest(from, f)
	case protocol.TypeRes:
		s.complete(f)
	case protocol.TypeEvt:
		s.collectEvent(from, f)
	}
}

func (s *Server) writeTo(plugin string, f *protocol.Frame) error {
	s.mu.Lock()
	p := s.plugins[plugin]
	closed := s.closed
	s.mu.Unlock()
	if closed || p == nil {
		return fmt.Errorf("plugin %s not connected", plugin)
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	debugFrame("->", plugin, f)
	return protocol.WriteFrame(p.stdin, f)
}

// call sends a Host-initiated req to pluginName and waits for its res (with timeout).
// Retries once on plugin_down after on-demand restart (crash recovery).
// Internal primitive (ADR-0027): exported callers go through CallByCap or
// CallByFace so nothing bypasses the registry.
func (s *Server) call(pluginName string, f *protocol.Frame) (*protocol.Frame, error) {
	out, err := s.callStream(pluginName, f)
	if err != nil {
		return nil, err
	}
	return out.Frame, nil
}

// callByPlugin is Host-initiated cap.method to a named Plugin (bypasses
// unique-owner lookup). Internal primitive: exported callers go through
// CallByCap or CallByFace (ADR-0027).
func (s *Server) callByPlugin(pluginName, cap, method string, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	out, err := s.call(pluginName, &protocol.Frame{
		V:       protocol.Version,
		Type:    protocol.TypeReq,
		Cap:     cap,
		Method:  method,
		Payload: payload,
	})
	if err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Payload, nil
}

// callStream is call plus any evt frames attributed to the request id while waiting.
// Internal primitive (ADR-0027).
func (s *Server) callStream(pluginName string, f *protocol.Frame) (*CallResult, error) {
	return s.callStreamOn(pluginName, f, nil)
}

// callStreamOn is callStream with a live callback for each attributed evt (Render Medium).
// Callback runs on the Host read-loop goroutine; keep it fast and non-blocking.
func (s *Server) callStreamOn(pluginName string, f *protocol.Frame, onEvent func(*protocol.Frame)) (*CallResult, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 1 {
			if err := s.ensureAlive(pluginName); err != nil {
				return nil, lastErr
			}
		} else if err := s.ensureAlive(pluginName); err != nil {
			return nil, err
		}
		res, err := s.callOnce(pluginName, f, onEvent)
		if err != nil {
			lastErr = err
			continue
		}
		if res.Frame.Error != nil && res.Frame.Error.Code == "plugin_down" && attempt == 0 {
			lastErr = fmt.Errorf("%s", res.Frame.Error.Error())
			continue
		}
		return res, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("call failed")
	}
	return nil, lastErr
}

// Close shuts down every Plugin: stdin EOF, grace wait, then kill tree.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	debugf("close host plugins=%d", len(s.plugins))
	plugins := make([]*proc, 0, len(s.plugins))
	for _, p := range s.plugins {
		plugins = append(plugins, p)
	}
	job := s.job
	s.job = nil
	s.mu.Unlock()

	for _, p := range plugins {
		p.wmu.Lock()
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		p.wmu.Unlock()
	}
	deadline := time.Now().Add(DefaultShutdownGrace)
	for _, p := range plugins {
		if p.cmd == nil || p.cmd.Process == nil {
			continue
		}
		done := make(chan error, 1)
		go func(cmd *exec.Cmd) { done <- cmd.Wait() }(p.cmd)
		remain := time.Until(deadline)
		if remain < 0 {
			remain = 0
		}
		select {
		case <-done:
		case <-time.After(remain):
			killTree(p.cmd)
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}
	// Closing the Job Object (KILL_ON_JOB_CLOSE) reaps any stragglers,
	// including when Host itself is dying without a clean Close of each child.
	if job != nil {
		job.close()
	}
	return nil
}

func (s *Server) callOnce(pluginName string, f *protocol.Frame, onEvent func(*protocol.Frame)) (*CallResult, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("server closed")
	}
	to := DefaultCallTimeout
	if p := s.plugins[pluginName]; p != nil {
		to = p.timeout
	}
	s.seq++
	id := fmt.Sprintf("host-%d", s.seq)
	f.ID = id
	ch := make(chan *CallResult, 1)
	s.pending[id] = &wait{kind: waitHost, target: pluginName, ch: ch, onEvent: onEvent}
	s.mu.Unlock()

	if err := s.writeTo(pluginName, f); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(to):
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("timeout: call to %s timed out after %s", pluginName, to)
	}
}
