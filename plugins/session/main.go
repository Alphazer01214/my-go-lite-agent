package main

import (
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"session",
		"create",
		pluginsdk.NewHandler(handleJSON(p.handleCreate)).
			WithInfo("create", "create one session"),
	)
	p.sdk.Register(
		"session",
		"info",
		pluginsdk.NewHandler(handleJSON(p.handleInfo)).
			WithInfo("info", "read one session metadata"),
	)
	p.sdk.Register(
		"session",
		"append",
		pluginsdk.NewHandler(handleJSON(p.handleAppend)).
			WithInfo("append", "append facts to one session"),
	)
	p.sdk.Register(
		"session",
		"facts",
		pluginsdk.NewHandler(handleJSON(p.handleFacts)).
			WithInfo("facts", "read facts of one session"),
	)
	p.sdk.Register(
		"session",
		"list",
		pluginsdk.NewHandler(handleJSONOptional(p.handleList)).
			WithInfo("list", "list sessions of one workspace"),
	)

	p.sdk.Register(
		"workspace",
		"list",
		pluginsdk.NewHandler(handleJSONOptional(p.handleWorkspaceList)).
			WithInfo("list", "list registered workspaces"),
	)
	p.sdk.Register(
		"workspace",
		"create",
		pluginsdk.NewHandler(handleJSON(p.handleWorkspaceCreate)).
			WithInfo("create", "register and select a workspace directory"),
	)
	p.sdk.Register(
		"workspace",
		"update",
		pluginsdk.NewHandler(handleJSON(p.handleWorkspaceUpdate)).
			WithInfo("update", "rename one workspace"),
	)
	p.sdk.Register(
		"workspace",
		"select",
		pluginsdk.NewHandler(handleJSON(p.handleWorkspaceSelect)).
			WithInfo("select", "select one workspace"),
	)
	p.sdk.Register(
		"workspace",
		"delete",
		pluginsdk.NewHandler(handleJSON(p.handleWorkspaceDelete)).
			WithInfo("delete", "remove one empty workspace registration"),
	)
	p.sdk.Register(
		"workspace",
		"current",
		pluginsdk.NewHandler(handleJSONOptional(p.handleWorkspaceCurrent)).
			WithInfo("current", "read the selected workspace"),
	)
}

func main() {
	plugin := newPlugin()
	plugin.registerHandlers()
	defer plugin.close()

	if err := plugin.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
