package main

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

const (
	factTypeMessage           = "message"
	factTypeSystemPrompt      = "system_prompt"
	factTypeRuntimeContext    = "runtime_context"
	factTypeContextCompaction = "context_compaction"
	factTypeContextSnapshot   = "context_snapshot"
	factTypeUsage             = "usage"

	trackStatic  = "static"
	trackDynamic = "dynamic"

	configPath          = "config.json"
	defaultMCPTimeoutMS = 30000
	maxMCPTimeoutMS     = 600000

	mcpTransportStdio = "stdio"
	mcpTransportHTTP  = "http"
	mcpTransportSSE   = "sse"
)

type promptSection struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
	Track string `json:"track,omitempty"`
	Text  string `json:"text"`
}

type skillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// mcpEntry 对应 Claude mcpServers 里的单个 server。Name 取自映射键，不写入值内。
type mcpEntry struct {
	Name        string            `json:"-"`
	Description string            `json:"description,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
	Type        string            `json:"type,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	TimeoutMS   int               `json:"timeout_ms,omitempty"`
	Tools       []string          `json:"tools,omitempty"`
}

type config struct {
	PromptSections     []promptSection     `json:"prompt_sections"`
	SkillsGlobalDir    string              `json:"skills_global_dir,omitempty"`
	SkillsWorkspaceDir string              `json:"skills_workspace_dir,omitempty"`
	MCPEnabled         *bool               `json:"mcp_enabled,omitempty"`
	MCPTimeoutMS       int                 `json:"mcp_timeout_ms,omitempty"`
	CompactEnabled     *bool               `json:"compact_enabled,omitempty"`
	MaxContextTokens   int                 `json:"max_context_tokens,omitempty"`
	CompactThreshold   float64             `json:"compact_threshold,omitempty"`
	RetainRecentTokens int                 `json:"retain_recent_tokens,omitempty"`
	CompactMaxChars    int                 `json:"compact_max_chars,omitempty"`
	Skills             []skillEntry        `json:"skills,omitempty"`
	MCPServers         map[string]mcpEntry `json:"mcpServers,omitempty"`
	SkillsReadable     bool                `json:"-"`
}

type createPayload struct {
	Workspace string `json:"workspace"`
	Model     string `json:"model,omitempty"`
}

type streamPayload struct {
	Model   string               `json:"model,omitempty"`
	Purpose string               `json:"purpose,omitempty"`
	Meta    streamPayloadSession `json:"meta,omitempty"`
}

type streamPayloadSession struct {
	SessionID string `json:"session_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
}

type sessionInfo struct {
	SessionID string `json:"session_id"`
	Workspace string `json:"workspace"`
	Model     string `json:"model,omitempty"`
}

type factIn struct {
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Reasoning   string               `json:"reasoning,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage      `json:"meta,omitempty"`
}

type sessionFact struct {
	Seq         int                  `json:"seq"`
	Type        string               `json:"type"`
	Role        string               `json:"role,omitempty"`
	TextContent string               `json:"text_content,omitempty"`
	Reasoning   string               `json:"reasoning,omitempty"`
	Files       []pluginsdk.File     `json:"files,omitempty"`
	ToolCalls   []pluginsdk.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID  string               `json:"tool_call_id,omitempty"`
	Meta        json.RawMessage      `json:"meta,omitempty"`
	Timestamp   int64                `json:"timestamp"`
}

type factsReq struct {
	SessionID string   `json:"session_id"`
	Types     []string `json:"types,omitempty"`
}

type factsResp struct {
	Facts   []sessionFact `json:"facts"`
	NextSeq int           `json:"next_seq"`
	Total   int           `json:"total"`
}

type appendReq struct {
	SessionID string   `json:"session_id"`
	Facts     []factIn `json:"facts"`
}

type appendResp struct {
	SessionID string `json:"session_id"`
	Seqs      []int  `json:"seqs"`
	NextSeq   int    `json:"next_seq"`
}

type factMeta struct {
	Source string `json:"source"`
	Hash   string `json:"hash,omitempty"`
}

type snapshotMeta struct {
	Source                string   `json:"source"`
	SessionID             string   `json:"session_id"`
	Model                 string   `json:"model,omitempty"`
	SurfaceSeqs           []int    `json:"surface_seqs,omitempty"`
	SystemPromptHash      string   `json:"system_prompt_hash,omitempty"`
	RuntimeContextHash    string   `json:"runtime_context_hash,omitempty"`
	MessagesHash          string   `json:"messages_hash,omitempty"`
	MessageCount          int      `json:"message_count"`
	ToolCatalogGeneration int64    `json:"tool_catalog_generation,omitempty"`
	ToolCatalogHash       string   `json:"tool_catalog_hash,omitempty"`
	ToolNames             []string `json:"tool_names,omitempty"`
}

type usageMeta struct {
	TurnId           string `json:"turn_id,omitempty"`
	Step             int    `json:"step,omitempty"`
	Model            string `json:"model,omitempty"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

type toolListResp struct {
	Tools             []toolListTool `json:"tools"`
	CatalogGeneration int64          `json:"catalog_generation"`
}

type toolListTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolCatalog struct {
	Generation int64            `json:"generation"`
	Tools      []pluginsdk.Tool `json:"tools"`
	Names      []string         `json:"names"`
	Hash       string           `json:"hash"`
}
