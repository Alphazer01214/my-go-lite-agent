package main

import (
	"sync"
	"time"
)

const (
	approvedTTL   = 60 * time.Second
	approvedLimit = 512
)

type approvedCall struct {
	Capability string
	Method     string
	ExpiresTS  int64
}

type approvalStore struct {
	mu    sync.Mutex
	items map[string]approvedCall
}

func newApprovalStore() *approvalStore {
	return &approvalStore{items: map[string]approvedCall{}}
}

// remember marks one already-adjudicated tool.call so the provider seam it
// dispatches does not ask a second time or write a duplicate audit entry.
func (s *approvalStore) remember(call callContext, now time.Time) {
	if call.CallID == "" || call.Capability == "" || call.Method == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]approvedCall{}
	}
	s.pruneLocked(now)
	s.items[call.CallID] = approvedCall{
		Capability: call.Capability,
		Method:     call.Method,
		ExpiresTS:  now.Add(approvedTTL).UnixMilli(),
	}
	for len(s.items) > approvedLimit {
		for key := range s.items {
			delete(s.items, key)
			break
		}
	}
}

func (s *approvalStore) consume(call callContext, now time.Time) bool {
	if call.CallID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[call.CallID]
	if !ok {
		return false
	}
	if now.UnixMilli() >= item.ExpiresTS {
		delete(s.items, call.CallID)
		return false
	}
	if item.Capability != call.Capability || item.Method != call.Method {
		return false
	}
	delete(s.items, call.CallID)
	return true
}

func (s *approvalStore) pruneLocked(now time.Time) {
	for key, item := range s.items {
		if now.UnixMilli() >= item.ExpiresTS {
			delete(s.items, key)
		}
	}
}
