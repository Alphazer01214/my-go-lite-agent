package main

import (
	"encoding/json"
	"time"
)

const (
	configPath = "config.json"

	defaultRefreshTTLMS  = 2000
	defaultInfoTimeoutMS = 1500

	maxInfoParallel      = 8
	maxTools             = 256
	maxDescriptionBytes  = 8192
	maxParametersBytes   = 64 * 1024
	maxProviderInfoBytes = 4 << 20
	maxArgumentsBytes    = 1 << 20
	maxContentBytes      = 1 << 20
	maxMetaBytes         = 64 * 1024
)

type config struct {
	Capabilities  []string `json:"capabilities"`
	RefreshTTLMS  int      `json:"refresh_ttl_ms"`
	InfoTimeoutMS int      `json:"info_timeout_ms"`
}

func defaultConfig() config {
	return config{
		RefreshTTLMS:  defaultRefreshTTLMS,
		InfoTimeoutMS: defaultInfoTimeoutMS,
	}
}

func (c config) refreshTTL() time.Duration {
	return time.Duration(c.RefreshTTLMS) * time.Millisecond
}

func (c config) infoTimeout() time.Duration {
	return time.Duration(c.InfoTimeoutMS) * time.Millisecond
}

type diagnostic struct {
	Capability string `json:"capability,omitempty"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

type providerInfo struct {
	Capability string         `json:"capability"`
	Tools      []providerTool `json:"tools"`
}

type providerTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Method      string          `json:"method"`
}

type catalogTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Capability  string
	Method      string
}

type catalog struct {
	Generation  int64
	BuiltAt     time.Time
	Tools       []catalogTool
	ByName      map[string]catalogTool
	Diagnostics []diagnostic
}

type listTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Capability  string          `json:"capability"`
	Method      string          `json:"method"`
}

type listResponse struct {
	Tools             []listTool   `json:"tools"`
	CatalogGeneration int64        `json:"catalog_generation"`
	Diagnostics       []diagnostic `json:"diagnostics"`
}

type callRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

type providerCall struct {
	Arguments json.RawMessage `json:"arguments"`
	SessionID string          `json:"session_id,omitempty"`
	Workspace string          `json:"workspace,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

func (c *catalog) response() listResponse {
	tools := make([]listTool, 0, len(c.Tools))
	for _, tool := range c.Tools {
		tools = append(tools, listTool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Capability:  tool.Capability,
			Method:      tool.Method,
		})
	}
	diagnostics := append([]diagnostic(nil), c.Diagnostics...)
	if diagnostics == nil {
		diagnostics = []diagnostic{}
	}
	return listResponse{
		Tools:             tools,
		CatalogGeneration: c.Generation,
		Diagnostics:       diagnostics,
	}
}
