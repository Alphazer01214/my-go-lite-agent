package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	mcpProtocolVersion = "2024-11-05"
	mcpClientName      = "liteagent-prompter"
	mcpClientVersion   = "0.1.0"
	maxMCPTools        = 256
)

type mcpJSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type mcpMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *mcpJSONRPCError `json:"error,omitempty"`
}

type mcpToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type mcpToolCallResult struct {
	Content           []mcpContentBlock `json:"content"`
	IsError           bool              `json:"isError,omitempty"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
}

type mcpContentBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
}

type mcpClient struct {
	name      string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	encoder   *json.Encoder
	decoder   *json.Decoder
	writeMu   sync.Mutex
	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[string]chan mcpMessage
	done      chan struct{}
	closeOnce sync.Once
	errMu     sync.Mutex
	lastErr   string
	onClose   func(*mcpClient)
}

func newMCPClient(name string, entry mcpEntry, ctx context.Context) (*mcpClient, error) {
	cmd := exec.CommandContext(ctx, entry.Command, entry.Args...)
	if strings.TrimSpace(entry.Cwd) != "" {
		cmd.Dir = entry.Cwd
	}
	cmd.Env = mergedMCPEnv(entry.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	client := &mcpClient{
		name:    name,
		cmd:     cmd,
		stdin:   stdin,
		encoder: json.NewEncoder(stdin),
		decoder: json.NewDecoder(stdout),
		pending: make(map[string]chan mcpMessage),
		done:    make(chan struct{}),
	}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	go client.readLoop()
	go func() {
		_ = cmd.Wait()
		client.closeWithError(fmt.Errorf("mcp server %s exited", name))
	}()
	return client, nil
}

func mergedMCPEnv(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func (c *mcpClient) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    mcpClientName,
			"version": mcpClientVersion,
		},
	}
	if _, err := c.request(ctx, "initialize", params); err != nil {
		return err
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		return err
	}
	return nil
}

func (c *mcpClient) listTools(ctx context.Context) ([]mcpToolInfo, error) {
	tools := make([]mcpToolInfo, 0)
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.request(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var result struct {
			Tools      []mcpToolInfo `json:"tools"`
			NextCursor string        `json:"nextCursor,omitempty"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, fmt.Errorf("decode tools/list: %w", err)
		}
		for _, tool := range result.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				continue
			}
			tools = append(tools, tool)
			if len(tools) >= maxMCPTools {
				return tools, nil
			}
		}
		if result.NextCursor == "" {
			return tools, nil
		}
		cursor = result.NextCursor
	}
}

func (c *mcpClient) callTool(ctx context.Context, name string, arguments json.RawMessage) (mcpToolCallResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	raw, err := c.request(ctx, "tools/call", params)
	if err != nil {
		return mcpToolCallResult{}, err
	}
	var result mcpToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return mcpToolCallResult{}, fmt.Errorf("decode tools/call: %w", err)
	}
	return result, nil
}

func (c *mcpClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if ctx == nil {
		return nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	id := fmt.Sprintf("prompter-%d", c.nextID.Add(1))
	ch := make(chan mcpMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	message := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		message["params"] = params
	}
	if err := c.write(message); err != nil {
		c.removePending(id)
		return nil, err
	}

	select {
	case response := <-ch:
		if response.Error != nil {
			return nil, fmt.Errorf("mcp %s: %d %s", method, response.Error.Code, response.Error.Message)
		}
		if len(response.Result) == 0 {
			return json.RawMessage(`{}`), nil
		}
		return response.Result, nil
	case <-ctx.Done():
		c.removePending(id)
		go func() {
			_ = c.notify("notifications/cancelled", map[string]any{
				"requestId": id,
				"reason":    "cancelled",
			})
		}()
		return nil, ctx.Err()
	case <-c.done:
		c.removePending(id)
		return nil, fmt.Errorf("mcp server %s is not running", c.name)
	}
}

func (c *mcpClient) notify(method string, params any) error {
	message := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		message["params"] = params
	}
	return c.write(message)
}

func (c *mcpClient) write(message any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.done:
		return fmt.Errorf("mcp server %s is not running", c.name)
	default:
	}
	return c.encoder.Encode(message)
}

func (c *mcpClient) readLoop() {
	for {
		var message mcpMessage
		if err := c.decoder.Decode(&message); err != nil {
			c.closeWithError(fmt.Errorf("mcp server %s read: %w", c.name, err))
			return
		}
		if message.Method != "" {
			continue
		}
		key := mcpIDKey(message.ID)
		if key == "" {
			continue
		}
		c.pendingMu.Lock()
		ch := c.pending[key]
		delete(c.pending, key)
		c.pendingMu.Unlock()
		if ch != nil {
			ch <- message
		}
	}
}

func (c *mcpClient) removePending(id string) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *mcpClient) closeWithError(err error) {
	c.closeOnce.Do(func() {
		if err != nil {
			c.errMu.Lock()
			c.lastErr = err.Error()
			c.errMu.Unlock()
		}
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		close(c.done)
		c.pendingMu.Lock()
		c.pending = make(map[string]chan mcpMessage)
		c.pendingMu.Unlock()
		if c.onClose != nil {
			c.onClose(c)
		}
	})
}

func (c *mcpClient) errorString() string {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	return c.lastErr
}

func mcpIDKey(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	return string(raw)
}

func mcpToolContent(result mcpToolCallResult) string {
	parts := make([]string, 0, len(result.Content))
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
			}
		default:
			if len(block.Data) > 0 {
				parts = append(parts, fmt.Sprintf("[%s content: %s]", block.Type, string(block.Data)))
			} else {
				parts = append(parts, fmt.Sprintf("[%s content]", block.Type))
			}
		}
	}
	if len(parts) == 0 && len(result.StructuredContent) > 0 {
		parts = append(parts, string(result.StructuredContent))
	}
	if len(parts) == 0 {
		return "(no output)"
	}
	return strings.Join(parts, "\n")
}
