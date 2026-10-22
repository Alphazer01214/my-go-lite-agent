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
}

type toolHandler func(context.Context, *pluginsdk.Request, ProviderInput, json.RawMessage) (json.RawMessage, error)

func handler(fn toolHandler) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		input, arguments, err := Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		return fn(ctx, req, input, arguments)
	}
}

func main() {
	sdk := pluginsdk.NewPlugin("file-tools")
	p := newPlugin()

	sdk.Register("file", "read", pluginsdk.NewHandler(handler(p.read)).WithInfo("read", "read a workspace file"))
	sdk.Register("file", "write", pluginsdk.NewHandler(handler(p.write)).WithInfo("write", "atomically write a workspace file"))
	sdk.Register("file", "list", pluginsdk.NewHandler(handler(p.list)).WithInfo("list", "list workspace entries"))
	sdk.Register("file", "stat", pluginsdk.NewHandler(handler(p.stat)).WithInfo("stat", "inspect a workspace path"))
	sdk.Register("file", "search", pluginsdk.NewHandler(handler(p.search)).WithInfo("search", "search workspace text"))
	sdk.Register("file", "glob", pluginsdk.NewHandler(handler(p.glob)).WithInfo("glob", "match workspace paths"))
	sdk.Register("file", "replace", pluginsdk.NewHandler(handler(p.replace)).WithInfo("replace", "replace literal text atomically"))
	sdk.Register("file", "patch", pluginsdk.NewHandler(handler(p.patch)).WithInfo("patch", "apply ordered literal edits atomically"))

	go func() {
		if err := SyncAll(context.Background(), sdk, definitions()); err != nil {
			fmt.Fprintln(os.Stderr, "file-tools register:", err)
		}
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type plugin struct{}

func newPlugin() *plugin { return &plugin{} }
