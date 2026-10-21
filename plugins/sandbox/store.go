package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// persistedRules is the on-disk shape of the rule file.
type persistedRules struct {
	Version int    `json:"version"`
	Rules   []rule `json:"rules"`
}

// ruleStore holds the rules of one plugin instance.
//
// Persistent rules live in the rule file and survive a restart. Session rules
// live in memory, bucketed by session id, and die with the process.
type ruleStore struct {
	mu         sync.RWMutex
	path       string
	version    int
	persistent []rule
	session    map[string][]rule
	seq        int
}

// newRuleStore loads the rule file. A broken file is skipped: the plugin
// starts with an empty rule set instead of refusing to run.
func newRuleStore(path string) *ruleStore {
	store := &ruleStore{path: path, session: map[string][]rule{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return store
	}
	var loaded persistedRules
	if err := json.Unmarshal(data, &loaded); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: rules: %v, starting with an empty rule set\n", err)
		return store
	}
	for _, item := range loaded.Rules {
		if !validAction(item.Action) || item.RuleID == "" || item.Scope.empty() {
			continue
		}
		if item.Lifetime == "" {
			item.Lifetime = LifetimePersistent
		}
		store.persistent = append(store.persistent, item)
		store.seq = max(store.seq, ruleSeq(item.RuleID))
	}
	store.version = loaded.Version
	return store
}

// ruleSeq reads the counter out of a rule id like "r-7".
func ruleSeq(ruleID string) int {
	if len(ruleID) < 2 || ruleID[0] != 'r' || ruleID[1] != '-' {
		return 0
	}
	value := 0
	for _, c := range ruleID[2:] {
		if c < '0' || c > '9' {
			return 0
		}
		value = value*10 + int(c-'0')
	}
	return value
}

// list returns the rules of one lifetime. A session lifetime needs the session
// id; an empty session id lists nothing.
func (s *ruleStore) list(lifetime, sessionID string) []rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch lifetime {
	case LifetimePersistent:
		out := make([]rule, len(s.persistent))
		copy(out, s.persistent)
		return out
	case LifetimeSession:
		if sessionID == "" {
			return nil
		}
		out := make([]rule, len(s.session[sessionID]))
		copy(out, s.session[sessionID])
		return out
	default:
		out := make([]rule, 0, len(s.persistent)+len(s.session[sessionID]))
		out = append(out, s.persistent...)
		out = append(out, s.session[sessionID]...)
		return out
	}
}

// activeRules returns the rules that apply to one call: the persistent ones,
// the session ones of that session, and none of the expired ones.
func (s *ruleStore) activeRules(sessionID string, now time.Time) []rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]rule, 0, len(s.persistent)+len(s.session[sessionID]))
	for _, item := range s.persistent {
		if expired(item, now) {
			continue
		}
		out = append(out, item)
	}
	for _, item := range s.session[sessionID] {
		if expired(item, now) {
			continue
		}
		out = append(out, item)
	}
	return out
}

// expired reports whether a rule outlived its time to live.
func expired(item rule, now time.Time) bool {
	if item.TTLSeconds <= 0 {
		return false
	}
	return now.UnixMilli()-item.CreatedTS > int64(item.TTLSeconds)*1000
}

// set adds or updates one rule. It returns the rule id, whether the rule was
// created and the new rule set version.
//
// The rule set is monotonic: a change that would weaken it is rejected with
// sandbox_rule_invalid.
func (s *ruleStore) set(next rule, sessionID string) (string, bool, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validAction(next.Action) {
		return "", false, s.version, ruleInvalid("action must be allow, modify or deny")
	}
	if next.Lifetime == "" {
		next.Lifetime = LifetimePersistent
	}
	if next.Lifetime != LifetimePersistent && next.Lifetime != LifetimeSession {
		return "", false, s.version, ruleInvalid("lifetime must be persistent or session")
	}
	if next.Lifetime == LifetimeSession && sessionID == "" {
		return "", false, s.version, ruleInvalid("session_id is required for a session rule")
	}
	if next.Scope.empty() {
		return "", false, s.version, ruleInvalid("scope must name at least one condition")
	}
	if next.Action == ActionModify && len(next.ModifiedArguments) == 0 {
		return "", false, s.version, ruleInvalid("modified_arguments is required for a modify rule")
	}
	if next.Priority == 0 {
		next.Priority = 100
	}
	if next.TTLSeconds < 0 {
		return "", false, s.version, ruleInvalid("ttl_seconds must not be negative")
	}
	if next.CreatedTS == 0 {
		next.CreatedTS = time.Now().UnixMilli()
	}

	bucket := s.persistent
	if next.Lifetime == LifetimeSession {
		bucket = s.session[sessionID]
	}

	index := -1
	for i, item := range bucket {
		if item.RuleID == next.RuleID {
			index = i
			break
		}
	}
	if index < 0 {
		if weakens(s.allLocked(next.Lifetime, sessionID), next) {
			return "", false, s.version, ruleInvalid("rule would weaken the rule set")
		}
		if next.RuleID == "" {
			// The counter only moves for a rule that is actually added, so a
			// rejected rule leaves no gap in the sequence.
			s.seq++
			next.RuleID = fmt.Sprintf("r-%d", s.seq)
		}
		if next.Lifetime == LifetimeSession {
			s.session[sessionID] = append(bucket, next)
		} else {
			s.persistent = append(bucket, next)
		}
		s.version++
		if err := s.persistLocked(); err != nil {
			return next.RuleID, true, s.version, err
		}
		return next.RuleID, true, s.version, nil
	}

	previous := bucket[index]
	if previous.Action == ActionDeny && next.Action != ActionDeny {
		return "", false, s.version, ruleInvalid("a deny rule cannot change its action")
	}
	others := s.allLocked(next.Lifetime, sessionID)
	filtered := make([]rule, 0, len(others))
	for _, item := range others {
		if item.RuleID == next.RuleID {
			continue
		}
		filtered = append(filtered, item)
	}
	if weakens(filtered, next) {
		return "", false, s.version, ruleInvalid("rule would weaken the rule set")
	}
	// An update keeps the creation time of the rule it replaces: the age of a
	// rule drives its time to live.
	next.CreatedTS = previous.CreatedTS
	bucket[index] = next
	if next.Lifetime == LifetimeSession {
		s.session[sessionID] = bucket
	} else {
		s.persistent = bucket
	}
	s.version++
	if err := s.persistLocked(); err != nil {
		return next.RuleID, false, s.version, err
	}
	return next.RuleID, false, s.version, nil
}

// delete removes one rule. A deny rule cannot be deleted.
func (s *ruleStore) delete(ruleID, lifetime, sessionID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := s.persistent
	if lifetime == LifetimeSession {
		bucket = s.session[sessionID]
	}
	for i, item := range bucket {
		if item.RuleID != ruleID {
			continue
		}
		if item.Action == ActionDeny {
			return s.version, ruleInvalid("a deny rule cannot be deleted")
		}
		remaining := append(bucket[:i:i], bucket[i+1:]...)
		if lifetime == LifetimeSession {
			s.session[sessionID] = remaining
		} else {
			s.persistent = remaining
		}
		s.version++
		if err := s.persistLocked(); err != nil {
			return s.version, err
		}
		return s.version, nil
	}
	return s.version, ruleNotFound("rule not found: " + ruleID)
}

// allLocked returns every rule of one lifetime. The caller holds the lock.
func (s *ruleStore) allLocked(lifetime, sessionID string) []rule {
	if lifetime == LifetimeSession {
		out := make([]rule, len(s.session[sessionID]))
		copy(out, s.session[sessionID])
		return out
	}
	out := make([]rule, len(s.persistent))
	copy(out, s.persistent)
	return out
}

// persistLocked writes the persistent rules to disk.
func (s *ruleStore) persistLocked() error {
	if s.path == "" {
		return nil
	}
	body, err := json.MarshalIndent(persistedRules{Version: s.version, Rules: s.persistent}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.path, body)
}

// sortForList orders rules for the list method: weight first, then rule id.
func sortForList(rules []rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].RuleID < rules[j].RuleID
	})
}
