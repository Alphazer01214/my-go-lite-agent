package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"os"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
	CallInContext(ctx context.Context, contextID, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

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
	sdk := pluginsdk.NewPlugin("tools")
	p := newToolsPlugin(sdk)

	sdk.Register("tools", "sync", pluginsdk.NewHandler(handleTyped(p.sync)).
		WithInfo("sync", "atomically replace the caller's complete tool set"))
	sdk.Register("tools", "register", pluginsdk.NewHandler(handleTyped(p.register)).
		WithInfo("register", "register or replace one tool"))
	sdk.Register("tools", "unregister", pluginsdk.NewHandler(handleTyped(p.unregister)).
		WithInfo("unregister", "remove one tool owned by the caller"))
	sdk.Register("tools", "list", pluginsdk.NewHandler(handleTyped(p.list)).
		WithInfo("list", "list currently available tools"))
	sdk.Register("tools", "call", pluginsdk.NewHandler(p.call).
		WithInfo("call", "dispatch one model tool call"))

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type toolsPlugin struct {
	sdk          sdkCaller
	reg          *registry
	registryPath string
}

func newToolsPlugin(sdk sdkCaller) *toolsPlugin {
	path := os.Getenv("TOOLS_REGISTRY_FILE")
	if path == "" {
		path = registryFile
	}
	reg, err := loadRegistry(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tools: load registry: %v\n", err)
	}
	return &toolsPlugin{sdk: sdk, reg: reg, registryPath: path}
}

func (p *toolsPlugin) persist() {
	if p.registryPath == "" {
		return
	}
	if err := p.reg.save(p.registryPath); err != nil {
		fmt.Fprintf(os.Stderr, "tools: persist registry: %v\n", err)
	}
}
