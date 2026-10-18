package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

func main() {
	sdk := pluginsdk.NewPlugin("sandbox")
	p := newSandbox(sdk)
	for _, method := range consumedMethods() {
		method := method
		sdk.Register(method.capability, method.method, pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
			return p.check(ctx, req)
		}).WithInfo(method.capability+"."+method.method, "sandbox policy check"))
	}
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type sandbox struct {
	sdk     sdkCaller
	policy  *policyStore
	methods map[string]string
}

func newSandbox(sdk sdkCaller) *sandbox {
	return &sandbox{
		sdk:    sdk,
		policy: newPolicyStore(permissionsPath()),
		methods: map[string]string{
			"file.write":    toolcontract.RiskHigh,
			"file.replace":  toolcontract.RiskHigh,
			"file.patch":    toolcontract.RiskHigh,
			"file.move":     toolcontract.RiskHigh,
			"file.delete":   toolcontract.RiskHigh,
			"shell.execute": toolcontract.RiskHigh,
			"web.fetch":     toolcontract.RiskMedium,
			"web.search":    toolcontract.RiskMedium,
		},
	}
}

type consumedMethod struct {
	capability string
	method     string
}

func consumedMethods() []consumedMethod {
	return []consumedMethod{
		{"tools", "call"},
		{"file", "write"},
		{"file", "replace"},
		{"file", "patch"},
		{"file", "move"},
		{"file", "delete"},
		{"shell", "execute"},
		{"web", "fetch"},
		{"web", "search"},
	}
}

func (p *sandbox) check(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	decision, err := p.authorize(ctx, req)
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, pluginsdk.ErrCode("sandbox_denied", decision.Reason)
	}
	if decision.Risk == toolcontract.RiskMedium {
		fmt.Fprintf(os.Stderr, "sandbox allow medium risk source=%s target=%s.%s tool=%s\n",
			req.From, decision.Capability, decision.Method, decision.Tool)
	}
	return req.Proceed(ctx, req.Payload)
}

type decision struct {
	Allowed    bool
	Risk       string
	Reason     string
	Capability string
	Method     string
	Tool       string
}

func (p *sandbox) authorize(ctx context.Context, req *pluginsdk.Request) (decision, error) {
	if req.Capability == "tools" && req.Method == "call" {
		return p.authorizeTool(ctx, req)
	}
	risk := p.methods[req.Capability+"."+req.Method]
	if risk == "" {
		return decision{Allowed: false, Reason: "unknown target is denied"}, nil
	}
	workspace := providerWorkspace(req.Payload)
	return p.policy.decide(ruleContext{
		Capability: req.Capability,
		Method:     req.Method,
		Risk:       risk,
		Workspace:  workspace,
		Source:     req.From,
	}), nil
}

func (p *sandbox) authorizeTool(ctx context.Context, req *pluginsdk.Request) (decision, error) {
	var in toolcontract.CallInput
	if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
		return decision{}, err
	}
	raw, err := p.toolsList(ctx, req.ContextID)
	if err != nil {
		return decision{Allowed: false, Reason: "cannot resolve tool risk"}, nil
	}
	var listed listOut
	if err := json.Unmarshal(raw, &listed); err != nil {
		return decision{Allowed: false, Reason: "cannot decode tool list"}, nil
	}
	for _, def := range listed.Tools {
		if def.Name != in.Name {
			continue
		}
		risk := toolcontract.NormalizeRisk(def.Risk)
		targetRisk := p.methods[def.TargetCapability+"."+def.TargetMethod]
		if riskRank(targetRisk) > riskRank(risk) {
			risk = targetRisk
		}
		if risk == "" {
			return decision{Allowed: false, Reason: "tool risk is unknown"}, nil
		}
		return p.policy.decide(ruleContext{
			Capability: def.TargetCapability,
			Method:     def.TargetMethod,
			Tool:       def.Name,
			Risk:       risk,
			Workspace:  in.Workspace,
			Source:     req.From,
		}), nil
	}
	return decision{Allowed: false, Reason: "tool is not registered"}, nil
}

func (p *sandbox) toolsList(ctx context.Context, contextID string) (json.RawMessage, error) {
	if contextID != "" {
		return p.sdk.CallInContext(ctx, contextID, "tools", "list", json.RawMessage(`{}`))
	}
	return p.sdk.Call(ctx, "tools", "list", json.RawMessage(`{}`))
}

func providerWorkspace(payload json.RawMessage) string {
	var in toolcontract.ProviderInput
	_ = json.Unmarshal(payload, &in)
	return in.Workspace
}

func riskRank(risk string) int {
	switch risk {
	case toolcontract.RiskLow:
		return 1
	case toolcontract.RiskMedium:
		return 2
	case toolcontract.RiskHigh:
		return 3
	default:
		return 0
	}
}

func permissionsPath() string {
	if value := os.Getenv("LITEAGENT_PERMISSIONS_FILE"); value != "" {
		return value
	}
	return filepathJoin("..", "..", "config", "permissions.json")
}
