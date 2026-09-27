package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func errBadArguments(msg string) error {
	return pluginsdk.ErrCode("bad_arguments", msg)
}

func errHandler(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, msg)
}

type event = pluginsdk.Event

func handle(fn func(req *pluginsdk.Request) (any, error)) pluginsdk.HandleFunc {
	return func(req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(req)
		if err != nil {
			return nil, err
		}
		if out == nil {
			return json.RawMessage("null"), nil
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, errHandler(err.Error())
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
	p := newPlugin()
	s := pluginsdk.NewPlugin("agent")
	p.sdk = s

	s.Register("loop", "turn", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in turnIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.turnWithEmit(req.ID, in)
	})).WithInfo("turn", "run one agent turn (multi-session concurrent)"))

	s.Register("loop", "cancel", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in cancelIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.cancel(in)
	})).WithInfo("cancel", "cancel in-flight turn for a session"))

	s.Register("agent", "config.get", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		return p.getConfig(), nil
	})).WithInfo("config.get", "show agent config"))

	s.Register("agent", "config.set", pluginsdk.NewHandler(handle(func(req *pluginsdk.Request) (any, error) {
		var in configSetIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return p.configSet(in)
	})).WithInfo("config.set", "update config (deferred while working)"))

	if err := s.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
