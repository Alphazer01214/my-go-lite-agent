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
}

func main() {
	sdk := pluginsdk.NewPlugin("web-tools")
	sdk.Register("web", "fetch", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		_, arguments, err := toolcontract.Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		var in fetchIn
		if err := toolcontract.DecodeStrict(arguments, &in); err != nil {
			return nil, err
		}
		return fetch(ctx, in)
	}).WithInfo("fetch", "fetch a public HTTP(S) resource"))
	go func() {
		if err := toolcontract.RegisterAll(context.Background(), sdk, definitions()); err != nil {
			fmt.Fprintln(os.Stderr, "web-tools register:", err)
		}
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func definitions() []toolcontract.Definition {
	return []toolcontract.Definition{{
		Name: "web_fetch", Risk: toolcontract.RiskMedium,
		TargetCapability: "web", TargetMethod: "fetch",
		Description: "Fetch a public HTTP or HTTPS resource with SSRF protection, redirect checks and a response size limit. " +
			"Only textual UTF-8 content is returned.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"url":{"type":"string","minLength":1},
				"method":{"type":"string","enum":["GET","HEAD"]},
				"headers":{"type":"object","additionalProperties":{"type":"string"}},
				"max_bytes":{"type":"integer","minimum":1,"maximum":4194304},
				"timeout_ms":{"type":"integer","minimum":1,"maximum":120000}
			},
			"required":["url"],
			"additionalProperties":false
		}`),
	}}
}
