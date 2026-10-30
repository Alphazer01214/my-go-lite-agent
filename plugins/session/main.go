package main

import (
	"fmt"
	"os"
)

func (p *plugin) registerHandlers() {
	p.sdk.Register(
		"session",
		"create",
		p.handleCreate,
	)
	p.sdk.Register(
		"session",
		"info",
		p.handleInfo,
	)
	p.sdk.Register(
		"session",
		"append",
		p.handleAppend,
	)
	p.sdk.Register(
		"session",
		"update",
		p.handleUpdate,
	)
	p.sdk.Register(
		"session",
		"delete",
		p.handleDelete,
	)
	p.sdk.Register(
		"session",
		"facts",
		p.handleFacts,
	)
	p.sdk.Register(
		"session",
		"list",
		p.handleList,
	)

	p.sdk.Register(
		"workspace",
		"list",
		p.handleWorkspaceList,
	)
	p.sdk.Register(
		"workspace",
		"create",
		p.handleWorkspaceCreate,
	)
	p.sdk.Register(
		"workspace",
		"update",
		p.handleWorkspaceUpdate,
	)
	p.sdk.Register(
		"workspace",
		"select",
		p.handleWorkspaceSelect,
	)
	p.sdk.Register(
		"workspace",
		"delete",
		p.handleWorkspaceDelete,
	)
	p.sdk.Register(
		"workspace",
		"current",
		p.handleWorkspaceCurrent,
	)
	p.sdk.Register("session", "messages", p.handleMessages)
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
