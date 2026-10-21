package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	sdk := pluginsdk.NewPlugin("subagent")
	p := newPlugin(sdk)

	sdk.Register("subagent", "run", pluginsdk.NewHandler(p.handleRun).
		WithInfo("run", "run one delegated task in a child session and return its answer"))

	go func() {
		logSyncError(syncTools(context.Background(), sdk))
	}()
	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p *plugin) handleRun(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
	input, args, err := decodeProvider(req.Payload)
	if err != nil {
		return nil, err
	}
	var in runIn
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, errBadArguments(err.Error())
	}
	out, err := p.run(ctx, input, in)
	if err != nil {
		return nil, err
	}
	return mustJSON(out), nil
}
