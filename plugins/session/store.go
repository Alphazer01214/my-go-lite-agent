package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// store 活跃的 session（即已加载到内存的会话日志）。
// 文件是 append-only JSONL：首行 meta，其余为 fact；meta 修订行也允许追加。
type store struct {
	id   string
	path string

	mu      sync.Mutex
	f       *os.File
	meta    Meta
	factCnt int
	nextSeq int
	lastTS  int64
}

// createStore 新建日志并写入首行 meta。文件已存在时返回 os.IsExist 可识别的错误。
func createStore(path, id string, meta Meta) (*store, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	line, err := json.Marshal(metaRecord{Kind: RecordKindMeta, Meta: meta})
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if _, err := f.WriteString(string(line) + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &store{id: id, path: path, f: f, meta: meta, nextSeq: 1}, nil
}

// openStore 以 O_APPEND 打开已有日志并恢复计数。
// O_APPEND 必须保留：否则重启后的首次写入会从文件头覆盖旧 Fact。
func openStore(path, id string) (*store, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st := &store{id: id, path: path, f: f, nextSeq: 1}
	if err := st.recover(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return st, nil
}

// recover 解析首行 meta 与后续记录，重建 nextSeq、factCnt、lastTS。
// 坏行跳过并写 stderr，不让一个损坏行拖垮整个会话。
func (s *store) recover() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	first := true
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if first {
			meta, err := decodeMeta(line)
			if err != nil {
				return fmt.Errorf("session %s meta line %d: %w", s.id, i+1, err)
			}
			s.meta = meta
			if s.meta.SessionId == "" {
				s.meta.SessionId = s.id
			}
			first = false
			continue
		}
		isMeta, err := isMetaLine(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", s.id, i+1, err)
			continue
		}
		if isMeta {
			meta, err := decodeMeta(line)
			if err != nil {
				fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", s.id, i+1, err)
				continue
			}
			s.meta = meta
			if s.meta.SessionId == "" {
				s.meta.SessionId = s.id
			}
			continue
		}
		fact, err := decodeFact(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", s.id, i+1, err)
			continue
		}
		s.factCnt++
		if fact.Seq >= s.nextSeq {
			s.nextSeq = fact.Seq + 1
		}
		if fact.Timestamp > s.lastTS {
			s.lastTS = fact.Timestamp
		}
	}
	if first {
		return fmt.Errorf("session %s has no meta record", s.id)
	}
	return nil
}

func decodeMeta(line string) (Meta, error) {
	var rec metaRecord
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return Meta{}, err
	}
	return rec.Meta, nil
}

func decodeFact(line string) (Fact, error) {
	var rec factRecord
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return Fact{}, err
	}
	return rec.Fact, nil
}

// isMetaLine 按 kind 判断记录类型；缺少 kind 时用 seq 是否存在兜底。
func isMetaLine(line string) (bool, error) {
	var head struct {
		Kind string `json:"kind"`
		Seq  *int   `json:"seq"`
	}
	if err := json.Unmarshal([]byte(line), &head); err != nil {
		return false, err
	}
	return head.Kind == RecordKindMeta || (head.Kind == "" && head.Seq == nil), nil
}

// appendBatch 追加一批事实。整批序列化后一次写入；写入失败不推进序号。
func (s *store) appendBatch(ins []factIn) ([]int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil, s.nextSeq, fmt.Errorf("session is closed")
	}

	batchTS := time.Now().UnixMilli()
	first := s.nextSeq
	lastTS := s.lastTS
	seqs := make([]int, 0, len(ins))
	var b strings.Builder
	for i, in := range ins {
		ts := in.Timestamp
		if ts <= 0 {
			ts = batchTS
		}
		f := Fact{
			Seq:         first + i,
			Type:        in.Type,
			Role:        in.Role,
			TextContent: in.TextContent,
			Files:       in.Files,
			ToolCalls:   in.ToolCalls,
			ToolCallID:  in.ToolCallID,
			Meta:        in.Meta,
			Timestamp:   ts,
		}
		line, err := json.Marshal(factRecord{Kind: RecordKindFact, Fact: f})
		if err != nil {
			return nil, s.nextSeq, err
		}
		b.Write(line)
		b.WriteByte('\n')
		seqs = append(seqs, f.Seq)
		if ts > lastTS {
			lastTS = ts
		}
	}

	end, err := s.f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, s.nextSeq, err
	}
	if _, err := s.f.WriteString(b.String()); err != nil {
		// 回滚半截写入，避免下一条记录与残行粘成一行。
		_ = s.f.Truncate(end)
		return nil, s.nextSeq, err
	}

	s.nextSeq = first + len(seqs)
	s.factCnt += len(seqs)
	s.lastTS = lastTS
	return seqs, s.nextSeq, nil
}

// readFacts 读取全部 fact（不含 meta 行），并在同一把锁内返回当前 nextSeq。
func (s *store) readFacts() ([]Fact, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, s.nextSeq, err
	}
	facts := make([]Fact, 0, s.factCnt)
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		isMeta, err := isMetaLine(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", s.id, i+1, err)
			continue
		}
		if isMeta {
			continue
		}
		fact, err := decodeFact(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session: skip corrupt line %s:%d: %v\n", s.id, i+1, err)
			continue
		}
		facts = append(facts, fact)
	}
	return facts, s.nextSeq, nil
}

// snapshot 返回当前 meta 与计数快照。
func (s *store) snapshot() (Meta, int, int, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meta, s.factCnt, s.nextSeq, s.lastTS
}

// bindWorkspace 为旧会话补上 workspace 归属，只更新内存，不改写日志。
func (s *store) bindWorkspace(workspaceId, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meta.WorkspaceId == "" {
		s.meta.WorkspaceId = workspaceId
	}
	if s.meta.Workspace == "" {
		s.meta.Workspace = path
	}
}

func (s *store) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		_ = s.f.Close()
		s.f = nil
	}
}
