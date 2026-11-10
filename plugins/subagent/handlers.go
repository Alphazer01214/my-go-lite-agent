package main

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) handleInfo(ctx context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(p.toolInfo())
}

func (p *plugin) handleRun(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input providerInput
	if len(req.Payload) > 0 {
		if err := json.Unmarshal(req.Payload, &input); err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
		}
	}
	args := input.Arguments
	if len(bytes.TrimSpace(args)) == 0 {
		// Direct capability calls may pass the arguments object at the top level.
		args = req.Payload
	}
	var in runIn
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}

	result, err := p.run(ctx, req.ContextID, input, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleRuns(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input runsReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	result, err := p.listRuns(ctx, req.ContextID, input.SessionID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (p *plugin) handleSettingsGet(_ context.Context, _ *pluginsdk.Request) (json.RawMessage, error) {
	return json.Marshal(settingsGetResp{
		Values: settingsValues{MaxDepth: p.currentConfig().MaxDepth},
	})
}

func (p *plugin) handleSettingsSave(_ context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	var input settingsSaveReq
	if err := pluginsdk.Decode(req, &input); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadPayload, err.Error())
	}
	cfg := config{MaxDepth: input.Values.MaxDepth}
	if err := cfg.validate(); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeBadArguments, err.Error())
	}
	if err := saveConfig(configPath, cfg); err != nil {
		return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
	}
	p.setConfig(cfg)
	return json.Marshal(settingsSaveResp{Saved: true})
}
