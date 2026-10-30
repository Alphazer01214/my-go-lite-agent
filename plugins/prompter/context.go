package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) baselineFacts(workspace, model string) ([]factIn, error) {
	vars := promptVars(workspace, model)
	staticText, err := p.cfg.renderStatic(vars)
	if err != nil {
		return nil, err
	}
	runtimeText, err := p.cfg.renderRuntime(vars)
	if err != nil {
		return nil, err
	}
	return []factIn{
		newContextFact(factTypeSystemPrompt, pluginsdk.RoleSystem, staticText),
		newContextFact(factTypeRuntimeContext, pluginsdk.RoleUser, runtimeText),
	}, nil
}

func newContextFact(factType, role, text string) factIn {
	meta, _ := json.Marshal(factMeta{Source: "prompter", Hash: hashText(text)})
	return factIn{
		Type:        factType,
		Role:        role,
		TextContent: text,
		Meta:        meta,
	}
}

func latestFact(facts []sessionFact, factType string) *sessionFact {
	for i := len(facts) - 1; i >= 0; i-- {
		if facts[i].Type == factType {
			return &facts[i]
		}
	}
	return nil
}

func projectMessages(facts []sessionFact) []pluginsdk.ChatMessage {
	messages := make([]pluginsdk.ChatMessage, 0, len(facts)+2)
	if fact := latestFact(facts, factTypeSystemPrompt); fact != nil && fact.TextContent != "" {
		messages = append(messages, pluginsdk.ChatMessage{
			Seq:  fact.Seq,
			Role: pluginsdk.RoleSystem,
			Text: fact.TextContent,
		})
	}
	if fact := latestFact(facts, factTypeRuntimeContext); fact != nil && fact.TextContent != "" {
		messages = append(messages, pluginsdk.ChatMessage{
			Seq:  fact.Seq,
			Role: pluginsdk.RoleUser,
			Text: fact.TextContent,
		})
	}
	for _, fact := range facts {
		if fact.Type != factTypeMessage || fact.Role == "" {
			continue
		}
		messages = append(messages, pluginsdk.ChatMessage{
			Seq:        fact.Seq,
			Role:       fact.Role,
			Text:       fact.TextContent,
			Files:      fact.Files,
			ToolCalls:  fact.ToolCalls,
			ToolCallID: fact.ToolCallID,
			Timestamp:  fact.Timestamp,
		})
	}
	return messages
}

func changedBaselineFacts(facts []sessionFact, baseline []factIn) []factIn {
	changed := make([]factIn, 0, len(baseline))
	for _, want := range baseline {
		current := latestFact(facts, want.Type)
		if current == nil || current.TextContent != want.TextContent {
			changed = append(changed, want)
		}
	}
	return changed
}

func surfaceSeqs(messages []pluginsdk.ChatMessage) []int {
	seqs := make([]int, 0, len(messages))
	for _, message := range messages {
		if message.Seq > 0 {
			seqs = append(seqs, message.Seq)
		}
	}
	return seqs
}

func messageHash(messages []pluginsdk.ChatMessage) string {
	hash := sha256.New()
	for _, message := range messages {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\x00", message.Role, message.Text, message.ToolCallID)
		for _, call := range message.ToolCalls {
			fmt.Fprintf(hash, "%s\x00%s\x00", call.FunctionCall.Name, string(call.FunctionCall.Arguments))
		}
		hash.Write([]byte{0x01})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func estimateTokens(messages []pluginsdk.ChatMessage) int {
	total := 0
	for _, message := range messages {
		runes := len([]rune(message.Text))
		total += (runes + 3) / 4
		total += 4
	}
	return total
}
