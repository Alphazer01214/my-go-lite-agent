package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func toSDKError(err error) error {
	if err == nil {
		return nil
	}
	if c, ok := err.(coded); ok {
		return pluginsdk.ErrCode(c.code, c.msg)
	}
	return err
}

func handle(fn func(ctx context.Context, req *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(ctx, req)
		if err != nil {
			return nil, toSDKError(err)
		}
		if out == nil {
			return json.RawMessage("null"), nil
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
		}
		return b, nil
	}
}

func decode[T any](req *pluginsdk.Request, dst *T) error {
	if len(req.Payload) == 0 {
		return errBadArguments("payload is required")
	}
	if err := json.Unmarshal(req.Payload, dst); err != nil {
		return errBadArguments(err.Error())
	}
	return nil
}

func main() {
	p := newPlugin(dataDir())
	p.loadAll()

	s := pluginsdk.NewPlugin("session")
	p.sdk = s
	p.setUIPublisher(s.WebUI())

	s.Register("session", "create", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in createIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.create(ctx, in)
	})).WithInfo("create", "create a new session"))

	s.Register("session", "append", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in appendIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.append(ctx, in)
	})).WithInfo("append", "append facts to a session"))

	s.Register("session", "query", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in queryIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.query(in)
	})).WithInfo("query", "query session facts"))

	s.Register("session", "derive", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in deriveIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.derive(in)
	})).WithInfo("derive", "project Model Context messages from the log"))

	s.Register("session", "list", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		return p.list()
	})).WithInfo("list", "list sessions"))

	s.Register("session", "info", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in infoIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.info(in)
	})).WithInfo("info", "session metadata and counts"))

	s.Register("session", "state", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in stateIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.state(ctx, in)
	})).WithInfo("state", "read rebuildable UI state"))

	s.Register("session", "select", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in selectIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.selectSession(ctx, in)
	})).WithInfo("select", "select the session shown by the UI mounts"))

	s.Register("session", "stream.update", pluginsdk.NewHandler(handle(func(ctx context.Context, req *pluginsdk.Request) (any, error) {
		var in streamUpdateIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.updateStream(ctx, in)
	})).WithInfo("stream.update", "update transient assistant stream state"))

	if err := s.Serve(); err != nil {
		os.Exit(1)
	}
}
