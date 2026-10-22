package main

import (
	"sync"
	"time"
)

// approvedTTL bounds how long the marker from a tools.call approval can skip
// the implicit direct seam. The nested provider call starts immediately after
// tools dispatches, so a minute is far more than enough.
const approvedTTL = 60 * time.Second

const approvedLimit = 512

// approvedCall records that one tools.call already passed adjudication. The
// tools plugin forwards the same call_id on the direct provider seam; without
// this marker the direct seam would ask the user a second time.
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

// consume returns true once for the matching nested call.
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
	delete(s.items, call.CallID)
	if now.UnixMilli() >= item.ExpiresTS {
		return false
	}
	return item.Capability == call.Capability && item.Method == call.Method
}

func (s *approvalStore) pruneLocked(now time.Time) {
	for key, item := range s.items {
		if now.UnixMilli() >= item.ExpiresTS {
			delete(s.items, key)
		}
	}
}
