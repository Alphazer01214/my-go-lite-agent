package main

import (
	"encoding/json"
)

const (
	configFile = "config.json"

	defaultSystemPrompt = "You are LiteAgent, a local coding agent. Use tools when they are needed. " +
		"Never invent file contents. Keep file and command operations inside the workspace. " +
		"Report concrete results, errors and remaining risks."

	defaultMaxContextTokens   = 60000
	defaultCompactThreshold   = 0.8
	defaultRetainTokens       = 12000
	defaultCompactMaxChars    = 12000
	defaultAgentsMDMaxBytes   = 64 << 10
	defaultSnapshotLimit      = 128
	agentsMDName              = "AGENTS.md"
	runtimeContextHeader      = "Current runtime context. This snapshot supersedes earlier runtime-context snapshots."
	runtimeContextCleared     = "Current runtime context: none. Earlier runtime-context snapshots no longer apply."
	runtimeContextHeading     = "## "
	kindSystemPrompt          = "system_prompt"
	kindRuntimeContext        = "runtime_context"
	sourceModelContext        = "model-context"
	factTypeContextSnapshot   = "context_snapshot"
	factTypeContextSummary    = "context_summary"
	factTypeContextCompaction = "context_compaction"
	factTypeMessage           = "message"
	roleSystem                = "system"
	roleUser                  = "user"
	roleAssistant             = "assistant"
	roleTool                  = "tool"
	roleHost                  = "host"
)

type promptSection struct {
	Name  string `json:"name"`
	Order int    `json:"order,omitempty"`
	Text  string `json:"text"`
}

type config struct {
	PromptSections      []promptSection `json:"prompt_sections,omitempty"`
	SubagentSections    []promptSection `json:"subagent_sections,omitempty"`
	SystemPrompt        string          `json:"system_prompt,omitempty"`
	MaxContextTokens    int             `json:"max_context_tokens,omitempty"`
	CompactThreshold    float64         `json:"compact_threshold,omitempty"`
	RetainRecentTokens  int             `json:"retain_recent_tokens,omitempty"`
	CompactMaxChars     int             `json:"compact_max_chars,omitempty"`
	AgentsMDMaxBytes    int             `json:"agents_md_max_bytes,omitempty"`
	SnapshotMemoryLimit int             `json:"snapshot_memory_limit,omitempty"`
}

type sectionAudit struct {
	Name  string `json:"name"`
	Order int    `json:"order,omitempty"`
	Hash  string `json:"hash"`
}

type completeMeta struct {
	SessionID     string `json:"session_id"`
	TurnID        string `json:"turn_id,omitempty"`
	StepID        string `json:"step_id,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	Workspace     string `json:"workspace,omitempty"`
	Model         string `json:"model,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
}

type llmMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Risk        int             `json:"risk,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
}
