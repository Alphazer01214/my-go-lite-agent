package main

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

type streamChunk struct {
	Channel  string `json:"channel"`
	Delta    string `json:"delta"`
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type streamAccumulator struct {
	mu        sync.Mutex
	content   strings.Builder
	reasoning strings.Builder
	toolCalls map[int]*toolCall
}

const (
	streamFlushMaxPending = 24
	streamFlushInterval   = 100 * time.Millisecond
)

type streamFlushPolicy struct {
	pending int
	last    time.Time
}

func newStreamFlushPolicy() *streamFlushPolicy {
	return &streamFlushPolicy{}
}

func (p *streamFlushPolicy) observe(now time.Time) bool {
	if p.last.IsZero() {
		p.last = now
		p.pending = 1
		return true
	}
	p.pending++
	if p.pending < streamFlushMaxPending && now.Sub(p.last) < streamFlushInterval {
		return false
	}
	p.pending = 0
	p.last = now
	return true
}

func newStreamSequencer() *streamSequencer {
	return &streamSequencer{}
}

type streamSequencer struct {
	mu   sync.Mutex
	next int
}

func (s *streamSequencer) withNext(snapshot streamSnapshot) streamSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	snapshot.UpdateID = s.next
	return snapshot
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{toolCalls: make(map[int]*toolCall)}
}

func (a *streamAccumulator) apply(payload json.RawMessage) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	var chunk streamChunk
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return false
	}
	switch chunk.Channel {
	case "content":
		a.content.WriteString(chunk.Delta)
	case "reasoning":
		a.reasoning.WriteString(chunk.Delta)
	case "tool_call":
		call := a.toolCalls[chunk.Index]
		if call == nil {
			call = &toolCall{Type: "function"}
			a.toolCalls[chunk.Index] = call
		}
		if chunk.ID != "" {
			call.ID = chunk.ID
		}
		if chunk.Type != "" {
			call.Type = chunk.Type
		}
		if chunk.Function.Name != "" {
			call.Function.Name = chunk.Function.Name
		}
		if chunk.Function.Arguments != "" {
			call.Function.Arguments += chunk.Function.Arguments
		}
	default:
		return false
	}
	return true
}

func (a *streamAccumulator) message() streamMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.messageLocked()
}

func (a *streamAccumulator) messageLocked() streamMessage {
	toolCalls := make([]toolCall, 0, len(a.toolCalls))
	indexes := make([]int, 0, len(a.toolCalls))
	for index := range a.toolCalls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		if call := a.toolCalls[index]; call != nil {
			toolCalls = append(toolCalls, *call)
		}
	}
	return streamMessage{
		Role:      RoleAssistant,
		Content:   a.content.String(),
		Reasoning: a.reasoning.String(),
		ToolCalls: toolCalls,
	}
}

func (a *streamAccumulator) snapshot(sessionID, turnID, stepID, status string) streamSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return streamSnapshot{
		SessionID: sessionID,
		TurnID:    turnID,
		StepID:    stepID,
		Message:   a.messageLocked(),
		Status:    status,
	}
}

func (p *plugin) updateStream(snapshot streamSnapshot) error {
	_, err := p.sdk.Call("session", "stream.update", mustJSON(snapshot))
	return err
}
