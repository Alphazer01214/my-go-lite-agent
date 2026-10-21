package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashString(value string) string {
	return hashBytes([]byte(value))
}

func hashJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return hashBytes(data)
}

func interpolate(text string, vars map[string]string) (string, error) {
	var b strings.Builder
	for {
		start := strings.Index(text, "{{")
		if start < 0 {
			b.WriteString(text)
			break
		}
		end := strings.Index(text[start+2:], "}}")
		if end < 0 {
			b.WriteString(text)
			break
		}
		b.WriteString(text[:start])
		name := strings.TrimSpace(text[start+2 : start+2+end])
		value, ok := vars[name]
		if !ok {
			return "", errBadArguments("unknown prompt variable: {{" + name + "}}")
		}
		b.WriteString(value)
		text = text[start+2+end+2:]
	}
	return b.String(), nil
}

func renderPrompt(sections []promptSection, vars map[string]string) (string, []sectionAudit, error) {
	if len(sections) == 0 {
		return "", nil, errBadArguments("prompt_sections must not be empty")
	}
	ordered := append([]promptSection(nil), sections...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Order != ordered[j].Order {
			return ordered[i].Order < ordered[j].Order
		}
		return ordered[i].Name < ordered[j].Name
	})

	seen := make(map[string]bool, len(ordered))
	parts := make([]string, 0, len(ordered))
	audits := make([]sectionAudit, 0, len(ordered))
	for _, section := range ordered {
		name := strings.TrimSpace(section.Name)
		if name == "" {
			return "", nil, errBadArguments("prompt section name is required")
		}
		if seen[name] {
			return "", nil, errBadArguments("duplicate prompt section: " + name)
		}
		seen[name] = true
		text, err := interpolate(section.Text, vars)
		if err != nil {
			return "", nil, err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
		audits = append(audits, sectionAudit{Name: name, Order: section.Order, Hash: hashString(text)})
	}
	if len(parts) == 0 {
		return "", nil, errBadArguments("system prompt is empty")
	}
	return strings.Join(parts, "\n\n"), audits, nil
}

type runtimeSection struct {
	name    string
	title   string
	content string
}

func renderRuntimeContext(sections []runtimeSection) (string, []sectionAudit, bool) {
	parts := make([]string, 0, len(sections))
	audits := make([]sectionAudit, 0, len(sections))
	for _, section := range sections {
		content := strings.TrimSpace(section.content)
		if content == "" {
			continue
		}
		parts = append(parts, runtimeContextHeading+section.title+"\n"+content)
		audits = append(audits, sectionAudit{Name: section.name, Hash: hashString(content)})
	}
	if len(parts) == 0 {
		return runtimeContextCleared, nil, true
	}
	return runtimeContextHeader + "\n\n" + strings.Join(parts, "\n\n"), audits, false
}

func readAgentsMD(workspace string, maxBytes int) (string, bool, string) {
	if strings.TrimSpace(workspace) == "" || !filepath.IsAbs(workspace) {
		return "", false, "workspace_unavailable"
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return "", false, "agents_md_unreadable"
	}
	defer root.Close()
	data, err := root.ReadFile(agentsMDName)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, ""
		}
		return "", false, "agents_md_unreadable"
	}
	truncated := false
	if maxBytes > 0 && len(data) > maxBytes {
		data = data[:maxBytes]
		truncated = true
	}
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
		truncated = true
	}
	if len(data) == 0 {
		return "", truncated, ""
	}
	return string(data), truncated, ""
}

func toLLM(messages []derivedMessage) []llmMessage {
	out := make([]llmMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, llmMessage{
			Role:       message.Role,
			Content:    message.Content,
			ToolCalls:  message.ToolCalls,
			ToolCallID: message.ToolCallID,
		})
	}
	return out
}

func messagesHash(messages []llmMessage) string {
	return hashJSON(messages)
}

func toolsHash(tools []toolDefinition) string {
	if tools == nil {
		tools = []toolDefinition{}
	}
	return hashJSON(tools)
}

func estimateTokens(messages []llmMessage, tools []toolDefinition) int {
	body, err := json.Marshal(struct {
		Messages []llmMessage     `json:"messages"`
		Tools    []toolDefinition `json:"tools"`
	}{Messages: messages, Tools: tools})
	if err != nil {
		return 0
	}
	return (len(body) + 3) / 4
}

func estimateMessageTokens(message llmMessage) int {
	size := len(message.Content) + len(message.ToolCalls)
	return (size+3)/4 + 4
}

func snapshotSurface(messages []derivedMessage) []int {
	surface := make([]int, 0, len(messages))
	for _, message := range messages {
		if message.Seq > 0 {
			surface = append(surface, message.Seq)
		}
	}
	return surface
}
