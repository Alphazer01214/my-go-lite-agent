package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// sdkCaller is the slice of the SDK the plugin uses. It exists so the
// handlers can be tested without a Host.
type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

// plugin is the sandbox instance.
type plugin struct {
	sdk   sdkCaller
	audit *auditLog

	// cfg is the active configuration. It changes when the settings are
	// saved; the rules in flight keep the configuration they started with.
	mu  sync.RWMutex
	cfg config

	// configPath is the configuration file. It is the plugin working
	// directory in a real run.
	configPath string

	rules *ruleStore

	// riskMu guards the cached tool table.
	riskMu sync.Mutex
	risk   riskTable
}

// newPlugin builds the plugin instance and loads its configuration and rules.
func newPlugin(sdk sdkCaller) *plugin {
	cfg := loadConfig(configFile)
	return &plugin{
		sdk:        sdk,
		audit:      &auditLog{limit: auditRingLimit},
		cfg:        cfg,
		configPath: configFile,
		rules:      newRuleStore(cfg.RulesFile),
	}
}

// handleTyped wraps a typed handler into a plugin handler.
func handleTyped[T any](fn func(context.Context, *pluginsdk.Request) (T, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(ctx, req)
		if err != nil {
			return nil, err
		}
		body, err := json.Marshal(out)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
		}
		return body, nil
	}
}

func main() {
	sdk := pluginsdk.NewPlugin("sandbox")
	p := newPlugin(sdk)

	// The sandbox is a Consumer. It registers the keys it adjudicates; the
	// providers keep the route ownership, so the call still reaches them
	// after the sandbox proceeds.
	sdk.Register("tools", "call", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("call", "adjudicate one tool call"))
	sdk.Register("file", "write", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("write", "adjudicate one file write"))
	sdk.Register("file", "replace", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("replace", "adjudicate one file replace"))
	sdk.Register("file", "patch", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("patch", "adjudicate one file patch"))
	sdk.Register("shell", "execute", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("execute", "adjudicate one command execution"))

	sdk.Register("sandbox", "check", pluginsdk.NewHandler(handleTyped(p.check)).
		WithInfo("check", "dry-run one tool call without side effects"))
	sdk.Register("sandbox", "list_rules", pluginsdk.NewHandler(handleTyped(p.listRules)).
		WithInfo("list_rules", "list the rules in effect"))
	sdk.Register("sandbox", "set_rule", pluginsdk.NewHandler(handleTyped(p.setRule)).
		WithInfo("set_rule", "add, update or delete one rule"))
	sdk.Register("sandbox", "settings.get", pluginsdk.NewHandler(handleTyped(p.settingsGet)).
		WithInfo("settings.get", "read the plugin settings"))
	sdk.Register("sandbox", "settings.save", pluginsdk.NewHandler(handleTyped(p.settingsSave)).
		WithInfo("settings.save", "write the plugin settings"))

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// handleSeam adjudicates one call on a consumed seam.
//
// The three outcomes of this build:
//
//   - allow: the payload proceeds unchanged;
//   - modify: the arguments are rewritten and the payload proceeds;
//   - deny: the call short circuits and the model gets a tool result that
//     names the reason.
func (p *plugin) handleSeam(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var payload seamPayload
	if err := DecodeSeam(req.Payload, &payload); err != nil {
		return nil, err
	}
	return p.adjudicate(ctx, req, payload, func(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
		return req.Proceed(ctx, body)
	})
}

// seamProceed continues the pipeline with a payload. It exists so the
// adjudication can be tested without a Host.
type seamProceed func(ctx context.Context, payload json.RawMessage) (json.RawMessage, error)

// adjudicate is the seam handler without the SDK plumbing.
func (p *plugin) adjudicate(
	ctx context.Context,
	req *pluginsdk.Request,
	payload seamPayload,
	proceed seamProceed,
) (json.RawMessage, error) {
	call, risk, err := p.resolveCall(ctx, req, payload)
	if err != nil {
		return nil, err
	}
	verdict := p.decide(call, risk)
	p.writeAudit(ctx, verdict, call, payload.Arguments)

	switch verdict.Action {
	case ActionAllow:
		return proceed(ctx, req.Payload)
	case ActionModify:
		arguments, err := applyTemplate(payload.Arguments, verdict.ModifiedArguments)
		if err != nil {
			return nil, err
		}
		rewritten, err := withArguments(req.Payload, arguments)
		if err != nil {
			return nil, err
		}
		return proceed(ctx, rewritten)
	default:
		return denyResult(verdict)
	}
}

// resolveCall turns a seam payload into the view the rules are matched
// against, and returns the risk value of the call.
func (p *plugin) resolveCall(ctx context.Context, req *pluginsdk.Request, payload seamPayload) (callContext, int, error) {
	table := p.toolTable(ctx)
	call := callContext{
		SessionID: payload.SessionID,
		Workspace: payload.Workspace,
		CallID:    payload.CallID,
	}
	risk := UnknownRisk

	if req.Capability == "tools" && req.Method == "call" {
		call.Tool = payload.Name
		if entry, ok := table.lookup(payload.Name); ok {
			call.Capability = entry.TargetCapability
			call.Method = entry.TargetMethod
			risk = entry.Risk
		}
	} else {
		call.Capability = req.Capability
		call.Method = req.Method
		if entry, ok := table.lookupTarget(req.Capability, req.Method); ok {
			call.Tool = entry.Name
			risk = entry.Risk
		}
	}

	args, err := Arguments(payload.Arguments)
	if err != nil {
		return callContext{}, 0, err
	}
	switch call.Capability {
	case "file":
		call.Path = ArgumentString(args, "path")
	case "shell":
		call.Command = commandLine(args)
	}
	return call, risk, nil
}

// decide runs the policy over the rules that apply to the call.
func (p *plugin) decide(call callContext, risk int) decision {
	p.mu.RLock()
	cfg := p.cfg
	p.mu.RUnlock()
	now := time.Now()
	return evaluate(p.rules.activeRules(call.SessionID, now), cfg, call, risk, now)
}

// commandLine renders the command of a shell call as one line.
func commandLine(args map[string]json.RawMessage) string {
	command := ArgumentString(args, "command")
	extra := ArgumentStrings(args, "args")
	if len(extra) == 0 {
		return command
	}
	return strings.Join(append([]string{command}, extra...), " ")
}

// denyResult renders the short circuit answer of a denied call. It is a tool
// result, not a transport error, so the model sees the reason and can decide
// what to do next.
func denyResult(verdict decision) (json.RawMessage, error) {
	content := fmt.Sprintf("Tool call denied by policy.\nTool: %s\nRisk: %d\nReason: %s",
		verdict.Tool, verdict.Risk, verdict.Reason)
	if verdict.RuleID != "" {
		content += "\nRule: " + verdict.RuleID
	}
	meta := map[string]any{
		"code":              CodeDenied,
		"sandbox_decision":  verdict.Action,
		"rule_id":           verdict.RuleID,
		"reason":            verdict.Reason,
		"risk":              verdict.Risk,
		"tool":              verdict.Tool,
		"target_capability": verdict.Capability,
		"target_method":     verdict.Method,
	}
	return encodeResult(toolResult{Content: content, IsError: true, Meta: meta})
}
