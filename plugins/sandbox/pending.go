package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

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

func (s *pendingStore) remove(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, token)
}

// decide removes a live entry and hands the decision to the waiting call.
func (s *pendingStore) decide(token, decision string, now time.Time) (*pendingConfirm, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[token]
	if !ok {
		return nil, pluginsdk.ErrCode("sandbox_confirm_token_invalid",
			"confirmation token is unknown or already used")
	}
	delete(s.items, token)
	if now.UnixMilli() >= item.ExpiresTS {
		return item, pluginsdk.ErrCode(CodeConfirmExpired, "confirmation token expired")
	}
	select {
	case item.Decision <- decision:
	default:
		return nil, pluginsdk.ErrCode("sandbox_confirm_token_invalid",
			"confirmation token already has a decision")
	}
	return item, nil
}

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
			ArgumentsPreview: item.ArgumentsPreview,
			CreatedTS:        item.CreatedTS,
			ExpiresAt:        item.ExpiresTS,
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
