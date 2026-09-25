package main

import (
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

func handle(fn func(req *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
	return func(req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(req)
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

	s.Register("session", "create", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in createIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.create(in)
	})).WithInfo("create", "create a new session"))

	s.Register("session", "append", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in appendIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.append(in)
	})).WithInfo("append", "append facts to a session"))

	s.Register("session", "query", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in queryIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.query(in)
	})).WithInfo("query", "query session facts"))

	s.Register("session", "derive", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in deriveIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.derive(in)
	})).WithInfo("derive", "project Model Context messages from the log"))

	s.Register("session", "list", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		return p.list()
	})).WithInfo("list", "list sessions"))

	s.Register("session", "info", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in infoIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.info(in)
	})).WithInfo("info", "session metadata and counts"))

	if err := s.Serve(); err != nil {
		os.Exit(1)
	}
}
