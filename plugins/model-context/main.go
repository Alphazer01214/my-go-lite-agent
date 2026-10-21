package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func handle(fn func(context.Context, *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(ctx, req)
		if err != nil {
			return nil, err
		}
		if out == nil {
			return json.RawMessage("null"), nil
		}
		data, err := json.Marshal(out)
		if err != nil {
			return nil, errHandler(err.Error())
		}
		return data, nil
	}
}

func decodeStrict[T any](req *pluginsdk.Request, dst *T) error {
	raw := bytes.TrimSpace(req.Payload)
	if len(raw) == 0 {
		return errBadArguments("payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errBadArguments(err.Error())
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errBadArguments("unexpected trailing JSON value")
		}
		return errBadArguments(err.Error())
	}
	return nil
}

func main() {
	sdk := pluginsdk.NewPlugin("model-context")
	p := newPlugin(sdk)

	sdk.Register("llm", "complete", pluginsdk.NewHandler(p.complete).
		WithInfo("complete", "assemble model context and continue the completion pipeline"))

	sdk.Register("model-context", "build", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in buildIn
		if err := decodeStrict(req, &in); err != nil {
			return nil, err
		}
		return p.build(ctx, req.ContextID, in)
	})).WithInfo("build", "dry-run context assembly without writing facts"))

	sdk.Register("model-context", "inspect", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in inspectIn
		if err := decodeStrict(req, &in); err != nil {
			return nil, err
		}
		return p.inspect(ctx, req.ContextID, in)
	})).WithInfo("inspect", "inspect a snapshot or reconstruct the latest request by session"))

	sdk.Register("model-context", "compact", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in compactIn
		if err := decodeStrict(req, &in); err != nil {
			return nil, err
		}
		return p.compact(ctx, req.ContextID, in)
	})).WithInfo("compact", "append a deterministic context summary"))

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
