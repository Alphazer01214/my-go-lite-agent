package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

func main() {
	sdk := pluginsdk.NewPlugin("subagent")
	p := newPlugin(sdk)
	sdk.Register("subagent", "run", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		input, arguments, err := toolcontract.Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		var in runIn
		if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
			return nil, err
		}
		out, err := p.run(ctx, req.ContextID, input, in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("run", "run one synchronous child Agent turn"))
	sdk.Register("subagent", "cancel", pluginsdk.NewHandler(func(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in cancelIn
		if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
			return nil, err
		}
		return json.Marshal(p.cancel(in))
	}).WithInfo("cancel", "cancel a running subagent task"))
	sdk.Register("subagent", "status", pluginsdk.NewHandler(func(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in statusIn
		if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
			return nil, err
		}
		return json.Marshal(p.status(in))
	}).WithInfo("status", "read one subagent task snapshot"))
	sdk.Register("subagent", "list", pluginsdk.NewHandler(func(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(p.list())
	}).WithInfo("list", "list subagent tasks in this process"))
	go func() {
		if err := toolcontract.RegisterAll(context.Background(), sdk, definitions()); err != nil {
			fmt.Fprintln(os.Stderr, "subagent register:", err)
		}
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type config struct {
	MaxDepth      int `json:"max_depth"`
	MaxConcurrent int `json:"max_concurrent"`
}

func defaultConfig() config {
	return config{MaxDepth: 2, MaxConcurrent: 4}
}

type plugin struct {
	sdk   sdkCaller
	cfg   config
	seq   atomic.Int64
	sem   chan struct{}
	mu    sync.Mutex
	tasks map[string]*task
}

type task struct {
	TaskID          string     `json:"task_id"`
	ParentSessionID string     `json:"parent_session_id,omitempty"`
	ChildSessionID  string     `json:"child_session_id,omitempty"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	Error           string     `json:"error,omitempty"`
	DelegationDepth int        `json:"delegation_depth"`
	cancel          context.CancelFunc
}

func newPlugin(sdk sdkCaller) *plugin {
	cfg := loadConfig()
	return &plugin{sdk: sdk, cfg: cfg, sem: make(chan struct{}, cfg.MaxConcurrent), tasks: make(map[string]*task)}
}

func loadConfig() config {
	cfg := defaultConfig()
	data, err := os.ReadFile("config.json")
	if err != nil {
		return cfg
	}
	var raw config
	if json.Unmarshal(data, &raw) != nil {
		return cfg
	}
	if raw.MaxDepth > 0 {
		cfg.MaxDepth = raw.MaxDepth
	}
	if raw.MaxConcurrent > 0 {
		cfg.MaxConcurrent = raw.MaxConcurrent
	}
	return cfg
}

type runIn struct {
	Task            string   `json:"task"`
	Workspace       string   `json:"workspace,omitempty"`
	ParentSessionID string   `json:"parent_session_id,omitempty"`
	AllowedTools    []string `json:"allowed_tools,omitempty"`
	MaxSteps        int      `json:"max_steps,omitempty"`
}

type runOut struct {
	TaskID        string `json:"task_id"`
	SessionID     string `json:"session_id"`
	Reply         string `json:"reply"`
	Steps         int    `json:"steps"`
	Cancelled     bool   `json:"cancelled"`
	ContextID     string `json:"context_id,omitempty"`
	ParentSession string `json:"parent_session_id,omitempty"`
}

func (p *plugin) run(ctx context.Context, contextID string, input toolcontract.ProviderInput, in runIn) (runOut, error) {
	if in.Task == "" {
		return runOut{}, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, "task is required")
	}
	workspace := in.Workspace
	if workspace == "" {
		workspace = input.Workspace
	}
	depth := 1
	if in.ParentSessionID != "" {
		parentDepth, err := p.parentDepth(ctx, contextID, in.ParentSessionID)
		if err != nil {
			return runOut{}, err
		}
		depth = parentDepth + 1
	}
	if depth > p.cfg.MaxDepth {
		return runOut{}, pluginsdk.ErrCode("subagent_depth_exceeded",
			fmt.Sprintf("delegation depth %d exceeds max %d", depth, p.cfg.MaxDepth))
	}
	select {
	case p.sem <- struct{}{}:
		defer func() { <-p.sem }()
	case <-ctx.Done():
		return runOut{}, pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "subagent queue cancelled")
	}

	taskID := fmt.Sprintf("sa-%d-%d", time.Now().UnixNano(), p.seq.Add(1))
	runCtx, cancel := context.WithCancel(ctx)
	entry := &task{
		TaskID: taskID, ParentSessionID: in.ParentSessionID,
		Status: "running", StartedAt: time.Now().UTC(), DelegationDepth: depth, cancel: cancel,
	}
	p.mu.Lock()
	p.tasks[taskID] = entry
	p.mu.Unlock()
	defer cancel()

	payload, _ := json.Marshal(map[string]any{
		"input": in.Task, "workspace": workspace, "parent_session_id": in.ParentSessionID,
		"origin": "subagent", "delegation_depth": depth, "allowed_tools": in.AllowedTools,
		"denied_tools": []string{"subagent_run", "subagent_cancel"}, "max_steps": in.MaxSteps,
	})
	raw, err := p.call(runCtx, contextID, "agent", "loop.turn", payload)
	now := time.Now().UTC()
	p.mu.Lock()
	entry.FinishedAt = &now
	if err != nil {
		if runCtx.Err() != nil {
			entry.Status = "cancelled"
			entry.Error = runCtx.Err().Error()
		} else {
			entry.Status = "failed"
			entry.Error = err.Error()
		}
		p.mu.Unlock()
		return runOut{}, err
	}
	var turn turnOut
	if err := json.Unmarshal(raw, &turn); err != nil {
		entry.Status = "failed"
		entry.Error = err.Error()
		p.mu.Unlock()
		return runOut{}, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "decode agent.loop.turn: "+err.Error())
	}
	entry.ChildSessionID = turn.SessionID
	if turn.Cancelled {
		entry.Status = "cancelled"
	} else {
		entry.Status = "completed"
	}
	p.mu.Unlock()
	return runOut{
		TaskID: taskID, SessionID: turn.SessionID, Reply: turn.Reply,
		Steps: turn.Steps, Cancelled: turn.Cancelled, ContextID: contextID,
		ParentSession: in.ParentSessionID,
	}, nil
}

type turnOut struct {
	SessionID string `json:"session_id"`
	Reply     string `json:"reply"`
	Steps     int    `json:"steps"`
	Cancelled bool   `json:"cancelled"`
}

type sessionInfoOut struct {
	DelegationDepth int `json:"delegation_depth"`
}

func (p *plugin) parentDepth(ctx context.Context, contextID, sessionID string) (int, error) {
	raw, err := p.call(ctx, contextID, "session", "info", mustJSON(map[string]string{"session_id": sessionID}))
	if err != nil {
		return 0, pluginsdk.ErrCode("subagent_parent_invalid", "parent session is unavailable: "+err.Error())
	}
	var info sessionInfoOut
	if err := json.Unmarshal(raw, &info); err != nil {
		return 0, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, "decode session.info: "+err.Error())
	}
	return info.DelegationDepth, nil
}

func (p *plugin) call(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error) {
	if contextID != "" {
		return p.sdk.CallInContext(ctx, contextID, capability, method, payload)
	}
	return p.sdk.Call(ctx, capability, method, payload)
}

type cancelIn struct {
	TaskID string `json:"task_id"`
}

type cancelOut struct {
	TaskID    string `json:"task_id"`
	Cancelled bool   `json:"cancelled"`
}

func (p *plugin) cancel(in cancelIn) cancelOut {
	p.mu.Lock()
	entry := p.tasks[in.TaskID]
	if entry != nil && entry.Status == "running" && entry.cancel != nil {
		entry.cancel()
		p.mu.Unlock()
		return cancelOut{TaskID: in.TaskID, Cancelled: true}
	}
	p.mu.Unlock()
	return cancelOut{TaskID: in.TaskID, Cancelled: false}
}

type statusIn struct {
	TaskID string `json:"task_id"`
}

func (p *plugin) status(in statusIn) task {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry := p.tasks[in.TaskID]; entry != nil {
		copy := *entry
		copy.cancel = nil
		return copy
	}
	return task{TaskID: in.TaskID, Status: "not_found"}
}

func (p *plugin) list() []task {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]task, 0, len(p.tasks))
	for _, entry := range p.tasks {
		copy := *entry
		copy.cancel = nil
		out = append(out, copy)
	}
	return out
}

func definitions() []toolcontract.Definition {
	return []toolcontract.Definition{{
		Name: "subagent_run", Risk: toolcontract.RiskMedium,
		TargetCapability: "subagent", TargetMethod: "run",
		Description: "Run a synchronous child Agent turn in a separate Session. " +
			"Use for bounded research, independent review or parallelizable analysis.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"task":{"type":"string","minLength":1},
				"workspace":{"type":"string"},
				"parent_session_id":{"type":"string"},
				"allowed_tools":{"type":"array","items":{"type":"string"}},
				"max_steps":{"type":"integer","minimum":1}
			},
			"required":["task"],
			"additionalProperties":false
		}`),
	}}
}

func mustJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return body
}
