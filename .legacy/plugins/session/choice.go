package main

import (
	"fmt"
	"sync"
	"time"
)

// defaultChoiceTimeout matches the old Medium-side approvalWait: it must stay
// under the enclosing Frame call timeout (serve.DefaultCallTimeout, 30s), or
// the caller's CallTo would die first and the structured {reason:"timeout"}
// res would be lost (fail-closed deny still holds, but callers lose context).
const defaultChoiceTimeout = 20 * time.Second

// maxChoiceTimeout keeps the ask res inside the enclosing 30s call timeout.
const maxChoiceTimeout = 25 * time.Second

// choiceOption is one selectable answer on a choice card (ADR-0034).
type choiceOption struct {
	Value  string `json:"value"`
	Label  string `json:"label,omitempty"`
	Danger bool   `json:"danger,omitempty"`
}

// choiceCenter tracks in-flight choice asks (ADR-0034): choice.ask registers
// and blocks; choice.respond resolves by id. First responder wins — late or
// unknown ids are rejected so a second Medium can never double-rule.
type choiceCenter struct {
	mu   sync.Mutex
	next int
	open map[string]chan string
}

func newChoiceCenter() *choiceCenter {
	return &choiceCenter{open: make(map[string]chan string)}
}

// register mints the next choice id and parks a resolve channel.
func (c *choiceCenter) register() (string, chan string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	id := fmt.Sprintf("choice-%d", c.next)
	ch := make(chan string, 1)
	c.open[id] = ch
	return id, ch
}

// resolve feeds the waiting ask; reports whether the id was still open.
func (c *choiceCenter) resolve(id, value string) bool {
	c.mu.Lock()
	ch, ok := c.open[id]
	if ok {
		delete(c.open, id)
	}
	c.mu.Unlock()
	if !ok {
		return false
	}
	ch <- value
	return true
}

// drop retires an ask that ended without a Medium answer (timeout).
func (c *choiceCenter) drop(id string) {
	c.mu.Lock()
	delete(c.open, id)
	c.mu.Unlock()
}

// choiceTimeout normalizes the caller-supplied bound.
func choiceTimeout(ms int) time.Duration {
	if ms <= 0 {
		return defaultChoiceTimeout
	}
	d := time.Duration(ms) * time.Millisecond
	if d > maxChoiceTimeout {
		return maxChoiceTimeout
	}
	return d
}
