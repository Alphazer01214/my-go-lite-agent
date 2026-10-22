package main

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

type store struct {
	mu        sync.Mutex
	id        string
	meta      Meta
	f         *os.File
	nextSeq   int
	factCount int
	lastTS    int64
}

func (s *store) snapshotMeta() Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meta
}

func (s *store) updateMeta(meta Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, _, err := s.appendLocked([]factIn{{
		Type: TypeSessionMeta,
		Role: RoleHost,
		Meta: mustJSON(meta),
	}}); err != nil {
		return err
	}
	s.meta = meta
	return nil
}

func (s *store) appendBatch(ins []factIn) ([]int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(ins)
}

func (s *store) appendLocked(ins []factIn) ([]int, int, error) {
	ts := nowMS()
	// The sequence is only consumed once the bytes are on the log: a failed
	// write must not leave a permanent gap that recover() would read back.
	firstSeq := s.nextSeq
	seqs := make([]int, 0, len(ins))
	facts := make([]Fact, 0, len(ins))
	for i, in := range ins {
		facts = append(facts, Fact{
			Seq:        firstSeq + i,
			Type:       in.Type,
			Role:       in.Role,
			Content:    in.Content,
			ToolCalls:  in.ToolCalls,
			ToolCallID: in.ToolCallID,
			Meta:       in.Meta,
			Timestamp:  ts,
		})
	}
	var b strings.Builder
	for _, f := range facts {
		line, err := json.Marshal(f)
		if err != nil {
			return nil, s.nextSeq, errHandler(err.Error())
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if _, err := s.f.WriteString(b.String()); err != nil {
		return nil, s.nextSeq, errHandler(err.Error())
	}
	for _, f := range facts {
		seqs = append(seqs, f.Seq)
	}
	s.nextSeq = firstSeq + len(facts)
	s.factCount += len(ins)
	s.lastTS = ts
	return seqs, s.nextSeq, nil
}

func (s *store) counts() (int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.factCount, s.lastTS
}

func (s *store) nextSequence() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextSeq
}
