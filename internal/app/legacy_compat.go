package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/serve"
)

// parseLegacyDomainFlags reads the removed domain argv (ADR-0030) only when
// L0_TEST_COMPAT=1. Product startup never exposes these; integration tests
// opt in so the L0 golden path can still drive session/loop diagnostics.
type legacyDomainFlags struct {
	sessionAppend string
	sessionDerive bool
	sessionQuery  bool
	agentRequest  string
	agentInject   string
	turnInput     string
	contextList   int
	cards         bool
	workspace     string
	invokePlugin  string
	invokePayload string
	frameCap      string
	frameMethod   string
	callCap       string
}

func loadLegacyDomainFlags() *legacyDomainFlags {
	if os.Getenv("L0_TEST_COMPAT") != "1" {
		return nil
	}
	args := os.Args[1:]
	get := func(name string) (string, bool) {
		for i, a := range args {
			if a == "-"+name {
				if i+1 < len(args) {
					return args[i+1], true
				}
				return "", true
			}
			if len(a) > len(name)+2 && a[:len(name)+2] == "-"+name+"=" {
				return a[len(name)+2:], true
			}
		}
		return "", false
	}
	has := func(name string) bool {
		for _, a := range args {
			if a == "-"+name {
				return true
			}
		}
		return false
	}
	f := &legacyDomainFlags{}
	f.sessionAppend, _ = get("session-append")
	f.sessionDerive = has("session-derive")
	f.sessionQuery = has("session-query")
	f.agentRequest, _ = get("agent-request")
	f.agentInject, _ = get("agent-inject")
	f.turnInput, _ = get("turn")
	if v, ok := get("context-list"); ok {
		f.contextList, _ = strconv.Atoi(v)
	}
	f.cards = has("cards")
	f.workspace, _ = get("workspace")
	if f.sessionAppend == "" && !f.sessionDerive && !f.sessionQuery &&
		f.agentRequest == "" && f.agentInject == "" && f.turnInput == "" &&
		f.contextList == 0 && !f.cards && f.workspace == "" {
		return nil
	}
	return f
}

// runLegacyDomain maps the test-compat argv onto L0 point-named calls.
func runLegacyDomain(pluginsDir, assemblyPath string, dump bool, f *legacyDomainFlags) error {
	plan, _, err := resolveAssembly(pluginsDir, assemblyPath, dump)
	if err != nil {
		return err
	}
	srv, err := startMounted(pluginsDir, plan)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()
	serve.RegisterApproval(srv, cliToolApproval)
	probeCommandFaces(srv, plan.Mounted)

	if f.workspace != "" {
		_, _ = callPlugin(srv, "session", "session", "create", map[string]any{
			"sessionId": "default", "workspace": f.workspace,
		})
	}
	if f.sessionAppend != "" {
		var facts []map[string]any
		if err := json.Unmarshal([]byte(f.sessionAppend), &facts); err != nil {
			return fmt.Errorf("parse -session-append: %w", err)
		}
		last := 0
		for _, fact := range facts {
			out, err := callPlugin(srv, "session", "session", "append", fact)
			if err != nil {
				return err
			}
			var res struct {
				Seq int `json:"seq"`
			}
			if len(out) > 0 {
				_ = json.Unmarshal(out, &res)
			}
			last = res.Seq
		}
		fmt.Printf("append ok count=%d lastSeq=%d\n", len(facts), last)
	}
	if f.agentInject != "" {
		var msgs []pluginsdk.Message
		if err := json.Unmarshal([]byte(f.agentInject), &msgs); err != nil {
			return fmt.Errorf("parse -agent-inject: %w", err)
		}
		// Deferred Host agent.inject special case (ADR-0030 #3).
		out, err := srv.AgentInject(serve.MarshalPayload(map[string]any{"messages": msgs}))
		if err != nil {
			return err
		}
		fmt.Printf("inject ok count=%d lastSeq=%d\n", out["count"], out["lastSeq"])
	}
	// -invoke runs before derive/query so side effects are visible.
	if f.invokePlugin != "" {
		payload := map[string]string{}
		if f.callCap != "" {
			payload["cap"] = f.callCap
		}
		rawPayload := serve.MarshalPayload(payload)
		if f.invokePayload != "" {
			rawPayload = json.RawMessage(f.invokePayload)
		}
		capName, methodName := "demo", "invoke"
		if f.frameCap != "" {
			capName = f.frameCap
		}
		if f.frameMethod != "" {
			methodName = f.frameMethod
		}
		out, err := srv.CallByPlugin(f.invokePlugin, capName, methodName, rawPayload)
		if err != nil {
			return fmt.Errorf("invoke %s: %w", f.invokePlugin, err)
		}
		fmt.Printf("invoke ok payload=%s\n", string(out))
	}
	if f.turnInput != "" {
		r := &turnRenderer{}
		restore := wireRenderer(srv, r)
		r.begin()
		out, err := runLoopTurn(srv, "", f.turnInput)
		if err != nil {
			r.end("")
			restore()
			return err
		}
		r.end(out.Assistant)
		restore()
		fmt.Printf("turn ok user=%s assistant=%q chunks=%d tools=%v\n",
			out.User, out.Assistant, len(out.Chunks), out.ToolCalls)
		for i, c := range out.Chunks {
			fmt.Printf("chunk[%d]=%q\n", i, c)
		}
		for i, name := range out.ToolCalls {
			fmt.Printf("tool_call[%d]=%s\n", i, name)
		}
		if u, err := callPlugin(srv, "context-manager", "context", "usage", map[string]any{}); err == nil && len(u) > 0 {
			fmt.Printf("context usage=%s\n", string(u))
		}
	}
	if f.sessionDerive {
		out, err := callPlugin(srv, "session", "session", "derive", map[string]any{})
		if err != nil {
			return err
		}
		var res struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Messages == nil {
			res.Messages = []json.RawMessage{}
		}
		body, _ := json.Marshal(res.Messages)
		fmt.Printf("derive ok messages=%s\n", body)
	}
	if f.sessionQuery {
		out, err := callPlugin(srv, "session", "session", "query", map[string]any{"afterSeq": 0, "limit": 0})
		if err != nil {
			return err
		}
		var res struct {
			Facts []json.RawMessage `json:"facts"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Facts == nil {
			res.Facts = []json.RawMessage{}
		}
		body, _ := json.Marshal(res.Facts)
		fmt.Printf("query ok facts=%s\n", body)
	}
	if f.contextList > 0 {
		out, err := callPlugin(srv, "context-manager", "context", "listContext", map[string]any{"n": f.contextList})
		if err != nil {
			return err
		}
		var res struct {
			Messages []json.RawMessage `json:"messages"`
			Count    int               `json:"count"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Messages == nil {
			res.Messages = []json.RawMessage{}
		}
		body, _ := json.Marshal(res.Messages)
		fmt.Printf("context list count=%d messages=%s\n", res.Count, body)
	}
	if f.agentRequest != "" {
		var claimed []pluginsdk.Message
		if err := json.Unmarshal([]byte(f.agentRequest), &claimed); err != nil {
			return fmt.Errorf("parse -agent-request: %w", err)
		}
		// Deferred Host agent.request special case (ADR-0030 #3).
		out, err := srv.AgentRequest("", claimed)
		if err != nil {
			return err
		}
		body, _ := json.Marshal(out)
		fmt.Printf("agent/request ok rebuilt=%v messages=%s\n", out.Rebuilt, body)
	}
	if f.cards {
		printCards(srv.Cards())
	}
	return nil
}

// pluginsdkMessage is a local alias so this file need not import pluginsdk
// just for inject parsing.
type pluginsdkMessage = struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}
