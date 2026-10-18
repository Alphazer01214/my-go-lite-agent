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

func handler(fn func(context.Context, *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
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
	sdk := pluginsdk.NewPlugin("model-context")
	p := newPlugin(sdk)
	sdk.Register("model-context", "build", pluginsdk.NewHandler(handler(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in buildIn
		if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
			return nil, err
		}
		return p.build(ctx, req.ContextID, in)
	})).WithInfo("build", "build and persist one Model Context Snapshot"))
	sdk.Register("model-context", "inspect", pluginsdk.NewHandler(handler(func(_ context.Context, req *pluginsdk.Request) (any, error) {
		var in inspectIn
		if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
			return nil, err
		}
		return p.inspect(in)
	})).WithInfo("inspect", "read a recently built Model Context Snapshot"))
	sdk.Register("model-context", "compact", pluginsdk.NewHandler(handler(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in compactIn
		if err := toolcontract.DecodeStrict(req.Payload, &in); err != nil {
			return nil, err
		}
		return p.compact(ctx, req.ContextID, in)
	})).WithInfo("compact", "append an active Session context summary"))
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
