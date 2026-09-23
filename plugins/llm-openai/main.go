package main

import "encoding/json"

type ChatModel struct {
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name       string `json:"name"`
		Arguments  string `json:"arguments"`
		Parameters []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"parameters"`
	} `json:"function"`
}

type tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type chatRequest struct {
}

type chatResponse struct {
}

type delta struct {
	ID      string `json:"id"`
	Choices []struct {
	}
}
