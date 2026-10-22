package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

// pendingConfirm is one call waiting for a human decision. The decision
// channel is buffered for exactly one delivery: sandbox.decide removes the
// entry before sending, so a second decision can never pair with it.
type pendingConfirm struct {
	Token            string
	Tool             string
	Capability       string
	Method           string
	SessionID        string
	RuleID           string
	Reason           string
	Risk             int
	ArgumentsPreview string
	ArgumentsDigest  string
	CreatedTS        int64
	ExpiresTS        int64
	Decision         chan string
}

type pendingStore struct {
	mu    sync.Mutex
	items map[string]*pendingConfirm
}

func newPendingStore() *pendingStore {
	return &pendingStore{items: map[string]*pendingConfirm{}}
}

func (s *pendingStore) add(item *pendingConfirm) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]*pendingConfirm{}
	}
	s.items[item.Token] = item
}

// remove takes one entry out of the store. It is safe to call after decide.
func (s *pendingStore) remove(token string) (*pendingConfirm, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[token]
	if ok {
		delete(s.items, token)
	}
	return item, ok
}

// decide removes a live entry and hands the decision to the waiting call. An
// expired entry is removed as well; the caller reports sandbox_confirm_expired.
func (s *pendingStore) decide(token, decision string, now time.Time) (*pendingConfirm, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[token]
	if !ok {
		return nil, confirmTokenInvalid("confirmation token is unknown or already used")
	}
	delete(s.items, token)
	if now.UnixMilli() >= item.ExpiresTS {
		return item, confirmExpired("confirmation token expired")
	}
	select {
	case item.Decision <- decision:
	default:
		return nil, confirmTokenInvalid("confirmation token already has a decision")
	}
	return item, nil
}

// list returns the live requests, optionally filtered to one session. Expired
// entries stay in the store until their waiting call cleans them up; they are
// not shown.
func (s *pendingStore) list(sessionID string, now time.Time) []pendingView {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]pendingView, 0, len(s.items))
	for _, item := range s.items {
		if now.UnixMilli() >= item.ExpiresTS {
			continue
		}
		if sessionID != "" && item.SessionID != sessionID {
			continue
		}
		out = append(out, pendingView{
			Token:            item.Token,
			Tool:             item.Tool,
			Capability:       item.Capability,
			Method:           item.Method,
			SessionID:        item.SessionID,
			RuleID:           item.RuleID,
			Reason:           item.Reason,
			Risk:             item.Risk,
			ArgumentsPreview: item.ArgumentsPreview,
			CreatedTS:        item.CreatedTS,
			ExpiresTS:        item.ExpiresTS,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedTS != out[j].CreatedTS {
			return out[i].CreatedTS < out[j].CreatedTS
		}
		return out[i].Token < out[j].Token
	})
	return out
}

func (s *pendingStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func newConfirmToken() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "cf-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("cf-%d", time.Now().UnixNano())
}
