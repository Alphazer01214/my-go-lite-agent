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

type config struct {
	AllowShellInterpreters bool `json:"allow_shell_interpreters"`
}

type plugin struct {
	cfg config
}

func loadConfig() config {
	var cfg config
	data, err := os.ReadFile("config.json")
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	return cfg
}

func main() {
	sdk := pluginsdk.NewPlugin("shell-tools")
	p := &plugin{cfg: loadConfig()}
	sdk.Register("shell", "execute", pluginsdk.NewHandler(func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		input, arguments, err := Payload(req.Payload)
		if err != nil {
			return nil, err
		}
		var in executeIn
		if err := DecodeStrict(arguments, &in); err != nil {
			return nil, err
		}
		return p.execute(ctx, input, in)
	}).WithInfo("execute", "execute one process without a shell"))
	go func() {
		if err := SyncAll(context.Background(), sdk, definitions(p.cfg)); err != nil {
			fmt.Fprintln(os.Stderr, "shell-tools register:", err)
		}
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
