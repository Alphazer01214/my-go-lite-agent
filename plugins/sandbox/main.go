package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"os"
	"sync"
	"time"
)

type sdkCaller interface {
	Call(ctx context.Context, capability, method string, payload json.RawMessage) (json.RawMessage, error)
}

type uiPublisher interface {
	SetState(mountID string, state json.RawMessage) error
}

type plugin struct {
	sdk   sdkCaller
	audit *auditLog
	ui    uiPublisher

	// cfg is the active configuration. It changes when the settings are
	// saved; the rules in flight keep the configuration they started with.
	mu  sync.RWMutex
	cfg config

	// configPath is the configuration file. It is the plugin working
	// directory in a real run.
	configPath string

	rules    *ruleStore
	pending  *pendingStore
	approved *approvalStore
	uiRev    uint64

	// riskMu guards the cached tool table.
	riskMu sync.Mutex
	risk   riskTable
}

func newPlugin(sdk sdkCaller) *plugin {
	cfg := loadConfig(configFile)
	return &plugin{
		sdk:        sdk,
		audit:      &auditLog{limit: auditRingLimit},
		cfg:        cfg,
		configPath: configFile,
		rules:      newRuleStore(cfg.RulesFile),
		pending:    newPendingStore(),
		approved:   newApprovalStore(),
		uiRev:      uint64(time.Now().UnixMicro()),
	}
}

func handleTyped[T any](fn func(context.Context, *pluginsdk.Request) (T, error)) pluginsdk.HandleFunc {
	return func(ctx context.Context, req *pluginsdk.Request) (json.RawMessage, error) {
		out, err := fn(ctx, req)
		if err != nil {
			return nil, err
		}
		body, err := json.Marshal(out)
		if err != nil {
			return nil, pluginsdk.ErrCode(pluginsdk.CodeHandlerError, err.Error())
		}
		return body, nil
	}
}

func main() {
	sdk := pluginsdk.NewPlugin("sandbox")
	p := newPlugin(sdk)
	p.ui = sdk.WebUI()

	// The sandbox is a Consumer. It registers the keys it adjudicates; the
	// providers keep the route ownership, so the call still reaches them
	// after the sandbox proceeds.
	sdk.Register("tools", "call", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("call", "adjudicate one tool call"))
	sdk.Register("file", "write", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("write", "adjudicate one file write"))
	sdk.Register("file", "replace", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("replace", "adjudicate one file replace"))
	sdk.Register("file", "patch", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("patch", "adjudicate one file patch"))
	sdk.Register("shell", "execute", pluginsdk.NewHandler(p.handleSeam).
		WithInfo("execute", "adjudicate one command execution"))

	sdk.Register("sandbox", "check", pluginsdk.NewHandler(handleTyped(p.check)).
		WithInfo("check", "dry-run one tool call without side effects"))
	sdk.Register("sandbox", "list_rules", pluginsdk.NewHandler(handleTyped(p.listRules)).
		WithInfo("list_rules", "list the rules in effect"))
	sdk.Register("sandbox", "set_rule", pluginsdk.NewHandler(handleTyped(p.setRule)).
		WithInfo("set_rule", "add, update or delete one rule"))
	sdk.Register("sandbox", "decide", pluginsdk.NewHandler(handleTyped(p.decideCall)).
		WithInfo("decide", "approve or reject one pending confirmation"))
	sdk.Register("sandbox", "pending.list", pluginsdk.NewHandler(handleTyped(p.listPending)).
		WithInfo("pending.list", "list calls waiting for confirmation"))
	sdk.Register("sandbox", "settings.get", pluginsdk.NewHandler(handleTyped(p.settingsGet)).
		WithInfo("settings.get", "read the plugin settings"))
	sdk.Register("sandbox", "settings.save", pluginsdk.NewHandler(handleTyped(p.settingsSave)).
		WithInfo("settings.save", "write the plugin settings"))

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
