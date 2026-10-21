package main

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type plugin struct {
	sdk sdkCaller
	cfg config

	mu            sync.Mutex
	snapshots     map[string]snapshot
	snapshotOrder []string

	revision atomic.Int64
	seq      atomic.Int64
}

func newPlugin(sdk sdkCaller) *plugin {
	return &plugin{sdk: sdk, cfg: loadConfig(), snapshots: make(map[string]snapshot)}
}

func errBadArguments(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeBadArguments, msg)
}

func errHandler(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, msg)
}

func errCoded(code, msg string) error {
	return pluginsdk.ErrCode(code, msg)
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}

func (p *plugin) call(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if contextID != "" {
		return p.sdk.CallInContext(ctx, contextID, capability, method, payload)
	}
	return p.sdk.Call(ctx, capability, method, payload)
}

func (p *plugin) derive(ctx context.Context, contextID, sessionID string) (deriveOut, error) {
	raw, err := p.call(ctx, contextID, "session", "derive", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return deriveOut{}, err
	}
	var out deriveOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return deriveOut{}, errCoded("context_derive_failed", "decode session.derive: "+err.Error())
	}
	if out.Messages == nil {
		out.Messages = []derivedMessage{}
	}
	return out, nil
}

func (p *plugin) appendFacts(ctx context.Context, contextID, sessionID string, facts []factIn) (appendOut, error) {
	raw, err := p.call(ctx, contextID, "session", "append", mustJSON(appendIn{SessionID: sessionID, Facts: facts}))
	if err != nil {
		return appendOut{}, errCoded("context_record_failed", err.Error())
	}
	var out appendOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return appendOut{}, errCoded("context_record_failed", "decode session.append: "+err.Error())
	}
	return out, nil
}

func (p *plugin) queryMessages(ctx context.Context, contextID, sessionID string) ([]fact, error) {
	raw, err := p.call(ctx, contextID, "session", "query", mustJSON(queryIn{
		SessionID: sessionID, Types: []string{factTypeMessage},
	}))
	if err != nil {
		return nil, errCoded("context_derive_failed", err.Error())
	}
	var out queryOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errCoded("context_derive_failed", "decode session.query: "+err.Error())
	}
	return out.Facts, nil
}

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

func (p *plugin) assemble(meta completeMeta, view deriveOut) (assembly, error) {
	vars := map[string]string{
		"workspace":    meta.Workspace,
		"workspace_id": meta.WorkspaceID,
		"mode":         meta.Mode,
		"scheme":       meta.Scheme,
	}
	prompt, sections, err := renderPrompt(p.cfg.PromptSections, vars)
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

// writeContextFacts appends only when the rendered content hash changed.
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
	asm, err := p.assemble(meta, view)
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

func (p *plugin) complete(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	if len(req.Payload) == 0 {
		return nil, errBadArguments("payload is required")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return nil, errBadArguments("llm.complete payload must be an object")
	}
	rawMeta, ok := payload["meta"]
	if !ok || len(rawMeta) == 0 {
		return nil, errBadArguments("meta is required")
	}
	var meta completeMeta
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		return nil, errBadArguments("invalid meta: " + err.Error())
	}
	if meta.SessionID == "" {
		return nil, errBadArguments("meta.session_id is required")
	}

	var tools []toolDefinition
	if rawTools, ok := payload["tools"]; ok && len(rawTools) > 0 {
		if err := json.Unmarshal(rawTools, &tools); err != nil {
			return nil, errBadArguments("tools must be an array: " + err.Error())
		}
	}

	out, err := p.prepare(ctx, req.ContextID, meta, tools)
	if err != nil {
		return nil, err
	}
	payload["messages"] = mustJSON(out.Messages)
	if _, ok := payload["tools"]; ok {
		payload["tools"] = mustJSON(out.Tools)
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return nil, errHandler("encode llm.complete payload: " + err.Error())
	}
	return req.Proceed(ctx, rewritten)
}

// contextLimit honors the configured cap and never exceeds a known model
// window. A zero window means the catalog had no metadata, so the cap stands.
func (p *plugin) contextLimit(contextWindow int) int {
	limit := p.cfg.MaxContextTokens
	if contextWindow > 0 && (limit <= 0 || contextWindow < limit) {
		return contextWindow
	}
	return limit
}

func (p *plugin) enforceBudget(
	ctx context.Context,
	contextID string,
	meta completeMeta,
	tools []toolDefinition,
	view deriveOut,
) (deriveOut, []string, error) {
	limit := p.contextLimit(meta.ContextWindow)
	if limit <= 0 {
		return view, nil, nil
	}
	estimate := estimateTokens(toLLM(view.Messages), tools)
	trigger := int(float64(limit) * p.cfg.CompactThreshold)
	if estimate < trigger {
		return view, nil, nil
	}

	warnings := make([]string, 0, 1)
	for range 3 {
		out, err := p.compactOnce(ctx, contextID, meta.SessionID, p.cfg.RetainRecentTokens, p.cfg.CompactMaxChars, "budget", view, tools)
		if err != nil {
			return view, warnings, err
		}
		if !out.Compacted {
			break
		}
		if !containsString(warnings, "compacted") {
			warnings = append(warnings, "compacted")
		}
		view, err = p.derive(ctx, contextID, meta.SessionID)
		if err != nil {
			return view, warnings, err
		}
		estimate = estimateTokens(toLLM(view.Messages), tools)
		if estimate < limit {
			return view, warnings, nil
		}
	}
	if estimate >= limit {
		return view, warnings, errCoded("context_budget_exceeded",
			fmt.Sprintf("context estimate %d exceeds max_context_tokens %d", estimate, limit))
	}
	return view, warnings, nil
}

type segment struct {
	facts  []fact
	tokens int
}

// selectSegments keeps the newest whole user segments that fit retainTokens and
// returns the older messages to summarize. It never splits a tool_call pair.
func selectSegments(ordinary []fact, retainTokens int) ([]fact, int) {
	if len(ordinary) == 0 {
		return nil, 0
	}
	segments := make([]segment, 0, len(ordinary))
	current := segment{}
	for _, item := range ordinary {
		if item.Role == roleUser && len(current.facts) > 0 {
			segments = append(segments, current)
			current = segment{}
		}
		current.facts = append(current.facts, item)
		current.tokens += estimateFactTokens(item)
	}
	if len(current.facts) > 0 {
		segments = append(segments, current)
	}

	keepFrom := len(segments)
	retained := 0
	for i := len(segments) - 1; i >= 0; i-- {
		if i < len(segments)-1 && retained+segments[i].tokens > retainTokens {
			break
		}
		keepFrom = i
		retained += segments[i].tokens
		if retained >= retainTokens {
			break
		}
	}
	if keepFrom == 0 {
		return nil, 0
	}
	summarized := make([]fact, 0)
	for _, seg := range segments[:keepFrom] {
		summarized = append(summarized, seg.facts...)
	}
	if len(summarized) == 0 {
		return nil, 0
	}
	return summarized, summarized[len(summarized)-1].Seq
}

func estimateFactTokens(item fact) int {
	return estimateMessageTokens(llmMessage{
		Role: item.Role, Content: item.Content, ToolCalls: item.ToolCalls,
	})
}

func ordinaryFacts(facts []fact) []fact {
	out := make([]fact, 0, len(facts))
	for _, item := range facts {
		if item.Type != factTypeMessage || item.Role == roleSystem {
			continue
		}
		meta := parseDerivedMeta(item.Meta)
		if meta.Kind == kindSystemPrompt || meta.Kind == kindRuntimeContext {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

func buildSummary(facts []fact, maxChars int) string {
	var b strings.Builder
	b.WriteString("Earlier conversation summary:\n")
	for _, item := range facts {
		content := strings.Join(strings.Fields(item.Content), " ")
		if len(content) > 400 {
			content = truncateUTF8(content, 400) + "..."
		}
		if content == "" && len(item.ToolCalls) == 0 {
			continue
		}
		b.WriteString("- ")
		b.WriteString(item.Role)
		b.WriteString(": ")
		b.WriteString(content)
		if len(item.ToolCalls) > 0 {
			b.WriteString(" [tool_calls]")
		}
		b.WriteByte('\n')
		if b.Len() >= maxChars {
			break
		}
	}
	value := b.String()
	if len(value) > maxChars {
		value = truncateUTF8(value, maxChars)
	}
	if strings.TrimSpace(value) == "Earlier conversation summary:" {
		return fmt.Sprintf("Earlier conversation summary:\n- %d messages omitted.", len(facts))
	}
	return value
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func estimateAfterCompaction(view deriveOut, summary string, cutoff int, tools []toolDefinition) int {
	messages := make([]llmMessage, 0, len(view.Messages)+1)
	inserted := false
	for _, message := range view.Messages {
		if view.SummarySeq != 0 && message.Seq == view.SummarySeq {
			continue
		}
		if message.Role != roleSystem && message.Seq <= cutoff {
			continue
		}
		messages = append(messages, llmMessage{
			Role: message.Role, Content: message.Content,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		})
		if message.Role == roleSystem && !inserted {
			messages = append(messages, llmMessage{Role: roleSystem, Content: summary})
			inserted = true
		}
	}
	if !inserted {
		messages = append([]llmMessage{{Role: roleSystem, Content: summary}}, messages...)
	}
	return estimateTokens(messages, tools)
}

func (p *plugin) compactOnce(
	ctx context.Context,
	contextID string,
	sessionID string,
	retainTokens int,
	maxChars int,
	reason string,
	view deriveOut,
	tools []toolDefinition,
) (compactOut, error) {
	out := compactOut{SessionID: sessionID}
	facts, err := p.queryMessages(ctx, contextID, sessionID)
	if err != nil {
		return out, err
	}
	ordinary := ordinaryFacts(facts)
	summarized, cutoff := selectSegments(ordinary, retainTokens)
	if cutoff == 0 || cutoff <= view.CoveredThroughSeq {
		return out, nil
	}

	summary := buildSummary(summarized, maxChars)
	sourceFrom := summarized[0].Seq
	compactionID := fmt.Sprintf("cc-%d-%d", time.Now().UnixNano(), p.seq.Add(1))
	tokenBefore := estimateTokens(toLLM(view.Messages), tools)
	tokenAfter := estimateAfterCompaction(view, summary, cutoff, tools)

	summaryPayload := summaryFactMeta{
		Active: true, CoversThroughSeq: cutoff,
		ShadowedFromSeq: sourceFrom, ShadowedToSeq: cutoff,
		PreviousSummary: view.SummarySeq,
		Plugin:          sourceModelContext, GeneratedBy: "model-context.compact",
		Strategy: "deterministic_digest", Reason: reason,
		CompactionID: compactionID, MessagesCovered: len(summarized),
	}
	compactionPayload := compactionFactMeta{
		Plugin: sourceModelContext, Reason: reason, Strategy: "deterministic_digest",
		CompactionID: compactionID, SourceFromSeq: sourceFrom, SourceToSeq: cutoff,
		Messages: len(summarized), TokenBefore: tokenBefore, TokenAfter: tokenAfter,
		PreviousSummary: view.SummarySeq,
	}
	appended, err := p.appendFacts(ctx, contextID, sessionID, []factIn{
		{Type: factTypeContextSummary, Role: roleHost, Content: summary, Meta: mustJSON(summaryPayload)},
		{Type: factTypeContextCompaction, Role: roleHost, Meta: mustJSON(compactionPayload)},
	})
	if err != nil {
		return out, err
	}
	summarySeq := 0
	if len(appended.Seqs) > 0 {
		summarySeq = appended.Seqs[0]
	}
	return compactOut{
		SessionID: sessionID, Compacted: true,
		SourceFromSeq: sourceFrom, SourceToSeq: cutoff, SummarySeq: summarySeq,
		Messages: len(summarized), TokenBefore: tokenBefore, TokenAfter: tokenAfter,
	}, nil
}

func (p *plugin) newSnapshot(meta completeMeta, view deriveOut, messages []llmMessage, tools []toolDefinition, warnings []string) snapshot {
	if tools == nil {
		tools = []toolDefinition{}
	}
	revision := int(p.revision.Add(1))
	return snapshot{
		ModelContextID: fmt.Sprintf("mc-%d-%d", time.Now().UnixNano(), revision),
		Revision:       revision,
		SessionID:      meta.SessionID,
		TurnID:         meta.TurnID,
		StepID:         meta.StepID,
		Mode:           meta.Mode,
		Model:          meta.Model,
		ContextWindow:  meta.ContextWindow,
		Surface:        snapshotSurface(view.Messages),
		MessageCount:   len(messages),
		MessagesHash:   messagesHash(messages),
		TruncatedSeqs:  append([]int(nil), view.TruncatedSeqs...),
		Tools:          tools,
		ToolsHash:      toolsHash(tools),
		TokenEstimate:  estimateTokens(messages, tools),
		Parts:          snapshotParts(view),
		Warnings:       dedupeStrings(warnings),
	}
}

func snapshotParts(view deriveOut) []snapshotPart {
	parts := make([]snapshotPart, 0, 4)
	var prompt, runtime, summary, session []int
	for _, message := range view.Messages {
		meta := parseDerivedMeta(message.Meta)
		switch {
		case meta.Kind == kindSystemPrompt:
			prompt = append(prompt, message.Seq)
		case meta.Kind == kindRuntimeContext:
			runtime = append(runtime, message.Seq)
		case view.SummarySeq != 0 && message.Seq == view.SummarySeq:
			summary = append(summary, message.Seq)
		default:
			session = append(session, message.Seq)
		}
	}
	if len(prompt) > 0 {
		parts = append(parts, snapshotPart{Source: "system_prompt", Seqs: prompt})
	}
	if len(summary) > 0 {
		parts = append(parts, snapshotPart{Source: "summary", Seqs: summary})
	}
	if len(runtime) > 0 {
		parts = append(parts, snapshotPart{Source: "runtime_context", Seqs: runtime})
	}
	if len(session) > 0 {
		parts = append(parts, snapshotPart{Source: "session", Seqs: session})
	}
	return parts
}

func (p *plugin) persistSnapshot(ctx context.Context, contextID string, snap snapshot) error {
	_, err := p.appendFacts(ctx, contextID, snap.SessionID, []factIn{{
		Type: factTypeContextSnapshot, Role: roleHost, Meta: mustJSON(snap),
	}})
	return err
}

func (p *plugin) rememberSnapshot(snap snapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshots[snap.ModelContextID] = snap
	p.snapshotOrder = append(p.snapshotOrder, snap.ModelContextID)
	limit := p.cfg.SnapshotMemoryLimit
	if limit <= 0 {
		limit = defaultSnapshotLimit
	}
	for len(p.snapshotOrder) > limit {
		oldest := p.snapshotOrder[0]
		p.snapshotOrder = p.snapshotOrder[1:]
		delete(p.snapshots, oldest)
	}
}

func (p *plugin) inspect(ctx context.Context, contextID string, in inspectIn) (inspectOut, error) {
	if in.ModelContextID == "" && in.SessionID == "" {
		return inspectOut{}, errBadArguments("model_context_id or session_id is required")
	}
	var snap snapshot
	if in.ModelContextID != "" {
		p.mu.Lock()
		found, ok := p.snapshots[in.ModelContextID]
		p.mu.Unlock()
		if !ok {
			return inspectOut{}, errCoded("model_context_not_found", "snapshot is not in the current process")
		}
		snap = found
	} else {
		facts, err := p.queryTypedFacts(ctx, contextID, in.SessionID, []string{
			factTypeMessage, factTypeContextSummary, factTypeContextSnapshot,
		})
		if err != nil {
			return inspectOut{}, err
		}
		found, ok := latestSnapshotFromFacts(facts)
		if !ok {
			return inspectOut{Found: false, Messages: []inspectedMessage{}}, nil
		}
		snap = found
	}

	out := inspectOut{
		Found:    true,
		Snapshot: snap,
		Messages: []inspectedMessage{},
	}
	if snap.SessionID == "" {
		out.Warnings = append(out.Warnings, "snapshot has no session_id; messages cannot be reconstructed")
		return out, nil
	}
	facts, err := p.queryTypedFacts(ctx, contextID, snap.SessionID, []string{
		factTypeMessage, factTypeContextSummary, factTypeContextSnapshot,
	})
	if err != nil {
		// The compact snapshot is still useful even when the log is unreadable.
		out.Warnings = append(out.Warnings, "reconstruction_unavailable")
		return out, nil
	}
	out.Messages, out.Breakdown, out.Warnings, out.Integrity = reconstructSnapshot(snap, facts)
	// Older snapshots predate truncated_seqs. The current derive can supply the
	// missing seqs; only accept the retry when the recorded hash proves it.
	if out.Integrity == "mismatch" && len(snap.TruncatedSeqs) == 0 {
		derived, err := p.derive(ctx, contextID, snap.SessionID)
		if err == nil && len(derived.TruncatedSeqs) > 0 {
			retry := snap
			retry.TruncatedSeqs = derived.TruncatedSeqs
			messages, breakdown, _, integrity := reconstructSnapshot(retry, facts)
			if integrity == "ok" {
				out.Messages = messages
				out.Breakdown = breakdown
				out.Integrity = "ok"
				kept := out.Warnings[:0]
				for _, warning := range out.Warnings {
					if warning != "messages_hash_mismatch" {
						kept = append(kept, warning)
					}
				}
				out.Warnings = append(kept, "truncation_inferred_from_current_derive")
			}
		}
	}
	return out, nil
}

func (p *plugin) queryTypedFacts(ctx context.Context, contextID, sessionID string, types []string) ([]fact, error) {
	raw, err := p.call(ctx, contextID, "session", "query", mustJSON(queryIn{
		SessionID: sessionID, Types: types,
	}))
	if err != nil {
		return nil, errCoded("context_derive_failed", err.Error())
	}
	var out queryOut
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errCoded("context_derive_failed", "decode session.query: "+err.Error())
	}
	return out.Facts, nil
}

func latestSnapshotFromFacts(facts []fact) (snapshot, bool) {
	var latest *fact
	for i := range facts {
		if facts[i].Type != factTypeContextSnapshot {
			continue
		}
		if latest == nil || facts[i].Seq > latest.Seq {
			latest = &facts[i]
		}
	}
	if latest == nil {
		return snapshot{}, false
	}
	var snap snapshot
	if len(latest.Meta) > 0 {
		_ = json.Unmarshal(latest.Meta, &snap)
	}
	if snap.ModelContextID == "" && len(snap.Surface) == 0 {
		return snapshot{}, false
	}
	return snap, true
}

// reconstructSnapshot rebuilds the exact request payload from surface seqs.
// Tool results that derive stubbed keep the same deterministic stub text, so
// the messages hash can verify the reconstruction against the recorded one.
func reconstructSnapshot(snap snapshot, facts []fact) ([]inspectedMessage, inspectBreakdown, []string, string) {
	bySeq := make(map[int]fact, len(facts))
	for _, item := range facts {
		if item.Type == factTypeMessage || item.Type == factTypeContextSummary {
			bySeq[item.Seq] = item
		}
	}
	truncated := make(map[int]bool, len(snap.TruncatedSeqs))
	for _, seq := range snap.TruncatedSeqs {
		truncated[seq] = true
	}
	messages := make([]inspectedMessage, 0, len(snap.Surface))
	llmMessages := make([]llmMessage, 0, len(snap.Surface))
	var breakdown inspectBreakdown
	warnings := make([]string, 0)
	integrity := "ok"
	for _, seq := range snap.Surface {
		item, ok := bySeq[seq]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("surface seq %d not found", seq))
			integrity = "mismatch"
			continue
		}
		role := item.Role
		source := "session"
		if item.Type == factTypeContextSummary {
			role = roleSystem
			source = "summary"
		} else {
			meta := parseDerivedMeta(item.Meta)
			switch meta.Kind {
			case kindSystemPrompt:
				source = "system_prompt"
			case kindRuntimeContext:
				source = "runtime_context"
			}
		}
		content := item.Content
		isTruncated := truncated[seq]
		if isTruncated {
			content = fmt.Sprintf("[tool result omitted; %d bytes; see seq=%d]", len(item.Content), item.Seq)
		}
		message := inspectedMessage{
			Seq: seq, Role: role, Content: content,
			ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID,
			Source: source, Truncated: isTruncated,
		}
		message.TokenEstimate = estimateMessageTokens(llmMessage{
			Role: role, Content: content, ToolCalls: item.ToolCalls, ToolCallID: item.ToolCallID,
		})
		switch source {
		case "system_prompt":
			breakdown.SystemPrompt += message.TokenEstimate
		case "runtime_context":
			breakdown.RuntimeContext += message.TokenEstimate
		case "summary":
			breakdown.Summary += message.TokenEstimate
		default:
			breakdown.Session += message.TokenEstimate
		}
		messages = append(messages, message)
	}
	// Rebuild the hashed form from the repaired messages: snapshots of older
	// sessions were recorded after the same orphan-tool repair in derive.
	repairInspectedToolMessages(messages)
	llmMessages = llmMessages[:0]
	for _, message := range messages {
		llmMessages = append(llmMessages, llmMessage{
			Role: message.Role, Content: message.Content,
			ToolCalls: message.ToolCalls, ToolCallID: message.ToolCallID,
		})
	}
	breakdown.Tools = estimateToolsTokens(snap.Tools)
	if snap.MessagesHash != "" && messagesHash(llmMessages) != snap.MessagesHash {
		warnings = append(warnings, "messages_hash_mismatch")
		integrity = "mismatch"
	}
	return messages, breakdown, warnings, integrity
}

func repairInspectedToolMessages(messages []inspectedMessage) {
	pending := map[string]bool{}
	for i := range messages {
		message := &messages[i]
		switch message.Role {
		case roleAssistant:
			pending = toolCallIDs(message.ToolCalls)
		case roleTool:
			if message.ToolCallID != "" && pending[message.ToolCallID] {
				delete(pending, message.ToolCallID)
				continue
			}
			prefix := fmt.Sprintf("[recovered tool result; original tool_call_id=%q]\n", message.ToolCallID)
			message.Content = strings.TrimRight(prefix+message.Content, "\n")
			message.Role = roleUser
			message.ToolCalls = nil
			message.ToolCallID = ""
		default:
			pending = map[string]bool{}
		}
	}
}

func toolCallIDs(raw json.RawMessage) map[string]bool {
	pending := map[string]bool{}
	if len(raw) == 0 {
		return pending
	}
	var calls []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return pending
	}
	for _, call := range calls {
		if call.ID != "" {
			pending[call.ID] = true
		}
	}
	return pending
}

func estimateToolsTokens(tools []toolDefinition) int {
	if len(tools) == 0 {
		return 0
	}
	body, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return (len(body) + 3) / 4
}

func (p *plugin) listTools(ctx context.Context, contextID string) ([]toolDefinition, []string) {
	raw, err := p.call(ctx, contextID, "tools", "list", json.RawMessage(`{}`))
	if err != nil {
		return []toolDefinition{}, []string{"tools_unavailable"}
	}
	var listed struct {
		Tools []toolSpec `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		return []toolDefinition{}, []string{"tools_unavailable"}
	}
	tools := make([]toolDefinition, 0, len(listed.Tools))
	for _, item := range listed.Tools {
		parameters := item.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		description := item.Description
		description += fmt.Sprintf("\nRisk: %d.", item.Risk)
		tools = append(tools, toolDefinition{
			Type: "function",
			Function: toolFunction{
				Name: item.Name, Description: description, Parameters: parameters,
			},
		})
	}
	return tools, nil
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

func (p *plugin) build(ctx context.Context, contextID string, in buildIn) (buildOut, error) {
	if in.SessionID == "" {
		return buildOut{}, errBadArguments("session_id is required")
	}
	view, err := p.derive(ctx, contextID, in.SessionID)
	if err != nil {
		return buildOut{}, err
	}
	tools := in.Tools
	warnings := []string{}
	if tools == nil {
		var toolWarnings []string
		tools, toolWarnings = p.listTools(ctx, contextID)
		warnings = append(warnings, toolWarnings...)
	}
	if tools == nil {
		tools = []toolDefinition{}
	}
	meta := completeMeta{
		SessionID: in.SessionID, TurnID: in.TurnID, StepID: in.StepID,
		Mode: in.Mode, Scheme: in.Scheme, Workspace: in.Workspace, WorkspaceID: in.WorkspaceID,
		Model: in.Model, ContextWindow: in.ContextWindow,
	}
	asm, err := p.assemble(meta, view)
	if err != nil {
		return buildOut{}, err
	}
	warnings = append(warnings, asm.Warnings...)
	messages := overlayView(view, asm)
	limit := p.contextLimit(in.ContextWindow)
	if in.MaxContextTokens > 0 {
		limit = in.MaxContextTokens
		if in.ContextWindow > 0 && in.ContextWindow < limit {
			limit = in.ContextWindow
		}
	}
	compactNeeded := limit > 0 && estimateTokens(messages, tools) >= int(float64(limit)*p.cfg.CompactThreshold)
	return buildOut{
		ModelContextID:     fmt.Sprintf("mc-preview-%d-%d", time.Now().UnixNano(), p.seq.Add(1)),
		Messages:           messages,
		Tools:              tools,
		TokenEstimate:      estimateTokens(messages, tools),
		CompactNeeded:      compactNeeded,
		Warnings:           dedupeStrings(warnings),
		SystemPromptHash:   asm.SystemHash,
		RuntimeContextHash: asm.RuntimeHash,
	}, nil
}

func (p *plugin) compact(ctx context.Context, contextID string, in compactIn) (compactOut, error) {
	if in.SessionID == "" {
		return compactOut{}, errBadArguments("session_id is required")
	}
	view, err := p.derive(ctx, contextID, in.SessionID)
	if err != nil {
		return compactOut{}, err
	}
	retain := p.cfg.RetainRecentTokens
	if in.KeepRecentTokens > 0 {
		retain = in.KeepRecentTokens
	}
	maxChars := p.cfg.CompactMaxChars
	if in.MaxSummaryChars > 0 {
		maxChars = in.MaxSummaryChars
	}
	tools := p.latestSnapshotTools(in.SessionID)
	return p.compactOnce(ctx, contextID, in.SessionID, retain, maxChars, "manual", view, tools)
}

func (p *plugin) latestSnapshotTools(sessionID string) []toolDefinition {
	p.mu.Lock()
	defer p.mu.Unlock()
	var best snapshot
	found := false
	for _, snap := range p.snapshots {
		if snap.SessionID != sessionID {
			continue
		}
		if !found || snap.Revision > best.Revision {
			best = snap
			found = true
		}
	}
	if !found {
		return []toolDefinition{}
	}
	return append([]toolDefinition(nil), best.Tools...)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || containsString(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}
