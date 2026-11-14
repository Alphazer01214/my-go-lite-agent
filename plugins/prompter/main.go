package main

import (
	"fmt"
	"os"
)

func main() {
	p := createPlugin()
	p.sdk.Register("session", "create", p.handleSessionCreate)
	p.sdk.Register("llm", "stream", p.handleStream)
	p.sdk.Register("prompter", "settings.get", p.handleSettingsGet)
	p.sdk.Register("prompter", "settings.save", p.handleSettingsSave)
	p.sdk.Register("prompter", "context.get", p.handleContextGet)
	p.sdk.Register("prompter", "compact", p.handleCompact)
	p.sdk.Register("prompter", "resources.list", p.handleResourcesList)
	p.sdk.Register("prompter", "resources.refresh", p.handleResourcesRefresh)
	p.sdk.Register("skill", "info", p.handleSkillInfo)
	p.sdk.Register("skill", "read", p.handleSkillRead)
	p.sdk.Register("mcp", "info", p.handleMCPInfo)
	p.sdk.Register("mcp", "call", p.handleMCPCall)
	p.sdk.Register("prompter", "mcp.config.get", p.handleMCPConfigGet)
	p.sdk.Register("prompter", "mcp.config.save", p.handleMCPConfigSave)

	p.startMCP()
	err := p.sdk.Serve()
	p.closeMCP()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
