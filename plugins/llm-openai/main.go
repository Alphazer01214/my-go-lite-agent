package main

import (
	"context"
	"encoding/json"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"os"
)

func errBadArguments(msg string) error {
	return pluginsdk.ErrCode("bad_arguments", msg)
}

func errUpstream(msg string) error {
	return errLlmUpstream(msg)
}

func errHandler(msg string) error {
	return pluginsdk.ErrCode(pluginsdk.CodeHandlerError, msg)
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

	s.Register("llm", "complete", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in completeIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		emit := func(payload json.RawMessage) error {
			return s.EmitWithID(req.ID, "llm", "chunk", payload)
		}
		out, err := p.complete(ctx, in, emit)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("complete", "OpenAI-compatible chat completion (stream by default)"))

	s.Register("llm", "cancel", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in llmCancelIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		return json.Marshal(llmCancelOut{CallID: in.CallID, Cancelled: p.cancelCall(in.CallID)})
	}).WithInfo("cancel", "cancel one in-flight completion"))
	s.Register("llm", "config.get", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(p.getConfig())
	}).WithInfo("config.get", "show model config (key masked)"))

	s.Register("llm", "config.set", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in configSetIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		out, err := p.configSet(in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("config.set", "update model config (applies immediately; running calls keep their snapshot)"))

	s.Register("llm", "models.list", pluginsdk.NewHandler(func(context.Context, *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(p.modelsList())
	}).WithInfo("models.list", "list configured models and the default"))

	s.Register("llm", "models.set", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in modelsSetIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		out, err := p.modelsSet(in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("models.set", "replace the model catalog and default"))

	s.Register("llm", "settings.get", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		return json.Marshal(p.settingsGet())
	}).WithInfo("settings.get", "read Settings Renderer values"))

	s.Register("llm", "settings.save", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		var in settingsSaveIn
		if err := decode(req, &in); err != nil {
			return nil, err
		}
		out, err := p.settingsSave(in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}).WithInfo("settings.save", "save Settings Renderer values"))

	if err := s.Serve(); err != nil {
		os.Exit(1)
	}
}
