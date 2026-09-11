// Command contextmanager is the Context Manager Plugin: provides system-prompt.
//
// Capability: system-prompt
//   - registerSegment: register one Prompt Segment {name, order, text}
//   - registerContext: register one dynamic context segment (v1 merged into text tail)
//   - assemble:        return ordered assembled System Prompt {text, segments}
//
// Optional static base segments load from segments.json beside the executable.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// Segment is one Prompt Segment (CONTEXT.md).
type Segment struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
	Text  string `json:"text"`
}

type store struct {
	mu       sync.Mutex
	segments map[string]Segment
	contexts map[string]Segment
}

func newStore() *store {
	return &store{
		segments: make(map[string]Segment),
		contexts: make(map[string]Segment),
	}
}

func (st *store) loadBaseFile() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "segments.json"))
	if err != nil {
		return
	}
	var file struct {
		Segments []Segment `json:"segments"`
		Contexts []Segment `json:"contexts"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, seg := range file.Segments {
		if seg.Name != "" {
			st.segments[seg.Name] = seg
		}
	}
	for _, seg := range file.Contexts {
		if seg.Name != "" {
			st.contexts[seg.Name] = seg
		}
	}
}

func (st *store) registerSegment(seg Segment) error {
	if seg.Name == "" {
		return &protocol.FrameError{Code: "bad_payload", Message: "name is required"}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.segments[seg.Name] = seg
	return nil
}

func (st *store) registerContext(seg Segment) error {
	if seg.Name == "" {
		return &protocol.FrameError{Code: "bad_payload", Message: "name is required"}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	st.contexts[seg.Name] = seg
	return nil
}

func sortByOrderName(items []Segment) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Order != items[j].Order {
			return items[i].Order < items[j].Order
		}
		return items[i].Name < items[j].Name
	})
}

func (st *store) assemble() (text string, segments []Segment) {
	st.mu.Lock()
	defer st.mu.Unlock()
	segments = make([]Segment, 0, len(st.segments))
	for _, seg := range st.segments {
		segments = append(segments, seg)
	}
	sortByOrderName(segments)

	ctx := make([]Segment, 0, len(st.contexts))
	for _, seg := range st.contexts {
		ctx = append(ctx, seg)
	}
	sortByOrderName(ctx)

	var parts []string
	for _, seg := range segments {
		if seg.Text != "" {
			parts = append(parts, seg.Text)
		}
	}
	for _, seg := range ctx {
		if seg.Text != "" {
			parts = append(parts, seg.Text)
		}
	}
	for i, p := range parts {
		if i > 0 {
			text += "\n\n"
		}
		text += p
	}
	return text, segments
}

func main() {
	s := pluginsdk.New()
	st := newStore()
	st.loadBaseFile()

	s.Handle("system-prompt", "registerSegment", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var seg Segment
		if err := json.Unmarshal(req.Payload, &seg); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if err := st.registerSegment(seg); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"name": seg.Name})
	})

	s.Handle("system-prompt", "registerContext", func(req *pluginsdk.Request) (json.RawMessage, error) {
		var seg Segment
		if err := json.Unmarshal(req.Payload, &seg); err != nil {
			return nil, &protocol.FrameError{Code: "bad_payload", Message: err.Error()}
		}
		if err := st.registerContext(seg); err != nil {
			return nil, err
		}
		return json.Marshal(map[string]string{"name": seg.Name})
	})

	s.Handle("system-prompt", "assemble", func(req *pluginsdk.Request) (json.RawMessage, error) {
		text, segments := st.assemble()
		return json.Marshal(map[string]any{
			"text":     text,
			"segments": segments,
		})
	})

	_ = s.Serve()
}
