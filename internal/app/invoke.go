package app

import (
	"encoding/json"
	"fmt"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/serve"
)

// invokeOpts is the L0 diagnostic surface (ADR-0030): one point-named call
// after mount. No domain flags — cap/method/payload come from argv generically.
type invokeOpts struct {
	pluginsDir    *string
	assemblyPath  *string
	dump          *bool
	invokePlugin  *string
	callCap       *string
	invokePayload *string
	frameCap      *string
	frameMethod   *string
}

func runInvoke(opts invokeOpts) error {
	plan, _, err := resolveAssembly(*opts.pluginsDir, *opts.assemblyPath, opts.dump != nil && *opts.dump)
	if err != nil {
		return err
	}
	srv, err := startMounted(*opts.pluginsDir, plan)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	serve.RegisterApproval(srv, cliToolApproval)
	probeCommandFaces(srv, plan.Mounted)

	payload := map[string]string{}
	if opts.callCap != nil && *opts.callCap != "" {
		payload["cap"] = *opts.callCap
	}
	var rawPayload json.RawMessage
	if opts.invokePayload != nil && *opts.invokePayload != "" {
		rawPayload = json.RawMessage(*opts.invokePayload)
	} else {
		rawPayload = serve.MarshalPayload(payload)
	}
	capName, methodName := "demo", "invoke"
	if opts.frameCap != nil && *opts.frameCap != "" {
		capName = *opts.frameCap
	}
	if opts.frameMethod != nil && *opts.frameMethod != "" {
		methodName = *opts.frameMethod
	}
	pluginName := *opts.invokePlugin

	// Paint loop.turn with the CLI render medium (pluginsdk display types, P2).
	isLoopTurn := capName == "loop" && methodName == "turn"
	var r *turnRenderer
	var restore func()
	if isLoopTurn {
		r = &turnRenderer{}
		restore = wireRenderer(srv, r)
		r.begin()
	}
	out, err := srv.CallByPlugin(pluginName, capName, methodName, rawPayload)
	if isLoopTurn && restore != nil {
		if err != nil {
			r.end("")
			restore()
			return err
		}
		var tr pluginsdk.TurnResult
		if len(out) > 0 {
			_ = json.Unmarshal(out, &tr)
		}
		r.end(tr.Assistant)
		restore()
		if tr.User == "" {
			var in struct {
				Input string `json:"input"`
			}
			_ = json.Unmarshal(rawPayload, &in)
			tr.User = in.Input
		}
		fmt.Printf("turn ok user=%s assistant=%q chunks=%d tools=%v\n",
			tr.User, tr.Assistant, len(tr.Chunks), tr.ToolCalls)
		for i, c := range tr.Chunks {
			fmt.Printf("chunk[%d]=%q\n", i, c)
		}
		for i, name := range tr.ToolCalls {
			fmt.Printf("tool_call[%d]=%s\n", i, name)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("invoke %s: %w", pluginName, err)
	}
	printInvokeResult(pluginName, capName, methodName, out)
	return nil
}

func printInvokeResult(pluginName, capName, method string, out json.RawMessage) {
	switch {
	case capName == "session" && method == "derive":
		var res struct {
			Messages []pluginsdk.Message `json:"messages"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Messages == nil {
			res.Messages = []pluginsdk.Message{}
		}
		body, _ := json.Marshal(res.Messages)
		fmt.Printf("derive ok messages=%s\n", body)
	case capName == "session" && method == "query":
		var res struct {
			Facts []map[string]any `json:"facts"`
		}
		if len(out) > 0 {
			_ = json.Unmarshal(out, &res)
		}
		if res.Facts == nil {
			res.Facts = []map[string]any{}
		}
		body, _ := json.Marshal(res.Facts)
		fmt.Printf("query ok facts=%s\n", body)
	case capName == "session" && method == "append":
		// May be a single object or used in a loop by tests via multiple invokes.
		fmt.Printf("append ok payload=%s\n", string(out))
	case capName == "session" && method == "list":
		fmt.Printf("list ok payload=%s\n", string(out))
	case capName == "session" && method == "current":
		fmt.Printf("current ok payload=%s\n", string(out))
	case capName == "session" && method == "create":
		fmt.Printf("create ok payload=%s\n", string(out))
	case capName == "agent" && method == "request":
		fmt.Printf("agent/request ok payload=%s\n", string(out))
	case capName == "agent" && method == "inject":
		fmt.Printf("inject ok payload=%s\n", string(out))
	case capName == "context" && method == "usage":
		fmt.Printf("context usage=%s\n", string(out))
	case capName == "context" && method == "listContext":
		fmt.Printf("context list payload=%s\n", string(out))
	case capName == "presentation" || method == "card":
		fmt.Printf("invoke ok payload=%s\n", string(out))
	default:
		fmt.Printf("invoke ok payload=%s\n", string(out))
	}
}
