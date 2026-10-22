package main

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
)

func parseDerivedMeta(raw json.RawMessage) derivedMeta {
	var meta derivedMeta
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &meta)
	}
	return meta
}

func findSystemPrompt(view deriveOut) (*derivedMessage, derivedMeta) {
	var found *derivedMessage
	var foundMeta derivedMeta
	for i := range view.Messages {
		message := &view.Messages[i]
		meta := parseDerivedMeta(message.Meta)
		if meta.Kind != kindSystemPrompt {
			continue
		}
		if found == nil || message.Seq > found.Seq {
			found = message
			foundMeta = meta
		}
	}
	return found, foundMeta
}

func findRuntimeContext(view deriveOut) (*derivedMessage, derivedMeta) {
	var found *derivedMessage
	var foundMeta derivedMeta
	for i := range view.Messages {
		message := &view.Messages[i]
		meta := parseDerivedMeta(message.Meta)
		if meta.Kind != kindRuntimeContext {
			continue
		}
		if found == nil || message.Seq > found.Seq {
			found = message
			foundMeta = meta
		}
	}
	return found, foundMeta
}

func findFallbackSystem(view deriveOut) *derivedMessage {
	for i := range view.Messages {
		message := &view.Messages[i]
		if message.Role != roleSystem {
			continue
		}
		if view.SummarySeq != 0 && message.Seq == view.SummarySeq {
			continue
		}
		meta := parseDerivedMeta(message.Meta)
		if meta.Kind == kindSystemPrompt || meta.Kind == kindRuntimeContext {
			continue
		}
		return message
	}
	return nil
}

func (p *plugin) assemble(meta completeMeta, view deriveOut, origin string) (assembly, error) {
	vars := map[string]string{
		"workspace":    meta.Workspace,
		"workspace_id": meta.WorkspaceID,
		"mode":         meta.Mode,
		"scheme":       meta.Scheme,
	}
	configured := p.cfg.PromptSections
	if strings.TrimSpace(origin) == "subagent" {
		configured = append(append([]promptSection(nil), configured...), p.cfg.SubagentSections...)
	}
	prompt, sections, err := renderPrompt(configured, vars)
	if err != nil {
		return assembly{}, err
	}
	if promptMsg, _ := findSystemPrompt(view); promptMsg == nil {
		if fallback := findFallbackSystem(view); fallback != nil {
			content := strings.TrimSpace(fallback.Content)
			if content != "" {
				prompt += "\n\nSession instructions:\n" + content
				sections = append(sections, sectionAudit{
					Name: "session_instructions", Hash: hashString(content),
				})
			}
		}
	}

	warnings := make([]string, 0, 2)
	runtimeSections := make([]runtimeSection, 0, 4)
	if workspace := strings.TrimSpace(meta.Workspace); workspace != "" {
		content := "Workspace: " + workspace
		if id := strings.TrimSpace(meta.WorkspaceID); id != "" {
			content += " (" + id + ")"
		}
		content += "\nPlatform: " + runtime.GOOS
		runtimeSections = append(runtimeSections, runtimeSection{
			name: "environment", title: "Environment", content: content,
		})
	}
	if strings.TrimSpace(meta.Mode) != "" {
		runtimeSections = append(runtimeSections, runtimeSection{
			name: "mode", title: "Mode", content: meta.Mode,
		})
	}
	if strings.TrimSpace(meta.Scheme) != "" {
		runtimeSections = append(runtimeSections, runtimeSection{
			name: "scheme", title: "Scheme", content: meta.Scheme,
		})
	}
	agents, truncated, warning := readAgentsMD(meta.Workspace, p.cfg.AgentsMDMaxBytes)
	if warning != "" {
		warnings = append(warnings, warning)
	}
	if truncated {
		warnings = append(warnings, "agents_md_truncated")
	}
	if agents != "" {
		runtimeSections = append(runtimeSections, runtimeSection{
			name: "agents_md", title: agentsMDName, content: agents,
		})
	}
	runtimeContent, runtimeSectionsAudit, cleared := renderRuntimeContext(runtimeSections)

	return assembly{
		SystemPrompt:    prompt,
		SystemHash:      hashString(prompt),
		SystemSections:  sections,
		RuntimeContext:  runtimeContent,
		RuntimeHash:     hashString(runtimeContent),
		RuntimeSections: runtimeSectionsAudit,
		RuntimeCleared:  cleared,
		Warnings:        warnings,
	}, nil
}

func currentMessageHash(message *derivedMessage, meta derivedMeta) string {
	if message == nil {
		return ""
	}
	if meta.Hash != "" {
		return meta.Hash
	}
	return hashString(message.Content)
}

func (p *plugin) writeContextFacts(
	ctx context.Context,
	contextID string,
	meta completeMeta,
	asm assembly,
	view deriveOut,
) (bool, error) {
	promptMsg, promptMeta := findSystemPrompt(view)
	runtimeMsg, runtimeMeta := findRuntimeContext(view)

	var facts []factIn
	if promptMsg == nil || currentMessageHash(promptMsg, promptMeta) != asm.SystemHash {
		facts = append(facts, factIn{
			Type: factTypeMessage, Role: roleSystem, Content: asm.SystemPrompt,
			Meta: mustJSON(systemFactMeta{
				Plugin: sourceModelContext, Source: sourceModelContext,
				Kind: kindSystemPrompt, Hash: asm.SystemHash, Sections: asm.SystemSections,
			}),
		})
	}

	writeRuntime := false
	switch {
	case runtimeMsg == nil:
		writeRuntime = !(asm.RuntimeCleared && len(asm.RuntimeSections) == 0)
	case runtimeMeta.Cleared && asm.RuntimeCleared:
		writeRuntime = false
	default:
		writeRuntime = currentMessageHash(runtimeMsg, runtimeMeta) != asm.RuntimeHash
	}
	if writeRuntime {
		facts = append(facts, factIn{
			Type: factTypeMessage, Role: roleUser, Content: asm.RuntimeContext,
			Meta: mustJSON(runtimeFactMeta{
				Plugin: sourceModelContext, Source: sourceModelContext,
				Kind: kindRuntimeContext, Hash: asm.RuntimeHash,
				Sections: asm.RuntimeSections, Cleared: asm.RuntimeCleared,
			}),
		})
	}

	if len(facts) == 0 {
		return false, nil
	}
	if _, err := p.appendFacts(ctx, contextID, meta.SessionID, facts); err != nil {
		return false, err
	}
	return true, nil
}

func (p *plugin) prepare(
	ctx context.Context,
	contextID string,
	meta completeMeta,
	tools []toolDefinition,
) (prepared, error) {
	if tools == nil {
		tools = []toolDefinition{}
	}
	view, err := p.derive(ctx, contextID, meta.SessionID)
	if err != nil {
		return prepared{}, err
	}
	origin, originWarnings := p.sessionOrigin(ctx, contextID, meta.SessionID)
	asm, err := p.assemble(meta, view, origin)
	if err != nil {
		return prepared{}, err
	}
	wrote, err := p.writeContextFacts(ctx, contextID, meta, asm, view)
	if err != nil {
		return prepared{}, err
	}
	if wrote {
		view, err = p.derive(ctx, contextID, meta.SessionID)
		if err != nil {
			return prepared{}, err
		}
	}

	warnings := append([]string(nil), asm.Warnings...)
	warnings = append(warnings, originWarnings...)
	view, compactWarnings, err := p.enforceBudget(ctx, contextID, meta, tools, view)
	if err != nil {
		return prepared{}, err
	}
	warnings = append(warnings, compactWarnings...)

	messages := toLLM(view.Messages)
	snap := p.newSnapshot(meta, view, messages, tools, warnings)
	if err := p.persistSnapshot(ctx, contextID, snap); err != nil {
		return prepared{}, err
	}
	p.rememberSnapshot(snap)
	return prepared{Messages: messages, Tools: tools, View: view, Snapshot: snap, Warnings: warnings}, nil
}

func overlayView(view deriveOut, asm assembly) []llmMessage {
	out := make([]llmMessage, 0, len(view.Messages)+2)
	out = append(out, llmMessage{Role: roleSystem, Content: asm.SystemPrompt})
	if view.SummarySeq != 0 {
		for _, message := range view.Messages {
			if message.Seq == view.SummarySeq {
				out = append(out, llmMessage{Role: roleSystem, Content: message.Content})
				break
			}
		}
	}
	if !asm.RuntimeCleared {
		out = append(out, llmMessage{Role: roleUser, Content: asm.RuntimeContext})
	}
	for _, message := range view.Messages {
		if message.Role == roleSystem {
			continue
		}
		meta := parseDerivedMeta(message.Meta)
		if meta.Kind == kindRuntimeContext {
			continue
		}
		out = append(out, llmMessage{
			Role: message.Role, Content: message.Content,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		})
	}
	return out
}
