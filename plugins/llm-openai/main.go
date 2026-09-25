package main

import (
	"encoding/json"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func errBadArguments(msg string) error {
	return pluginsdk.ErrCode("bad_arguments", msg)
}

func errUpstream(msg string) error {
	return pluginsdk.ErrCode("llm_upstream", msg)
}

func errHandler(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, msg)
}

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
	s := pluginsdk.NewPlugin("llm-openai")

	s.Register("llm", "complete", pluginsdk.NewHandler(func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in completeIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		emit := func(payload json.RawMessage) error {
			return s.EmitWithID(req.ID, "llm", "chunk", payload)
		}
		out, err := p.complete(in, emit)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("complete", "OpenAI-compatible chat completion (stream by default)"))

	s.Register("llm", "config.get", pluginsdk.NewHandler(func(req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(p.getConfig())
	}).WithInfo("config.get", "show model config (key masked)"))

	s.Register("llm", "config.set", pluginsdk.NewHandler(func(req *pluginsdk.Request) (json.RawMessage, error) {
		var in configSetIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		out, err := p.configSet(in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("config.set", "update config (deferred while working)"))

	if err := s.Serve(); err != nil {
		os.Exit(1)
	}
}
