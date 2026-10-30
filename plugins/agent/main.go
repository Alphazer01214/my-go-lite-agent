package main

import (
	"fmt"
	"os"
)

func (p *plugin) registerHandlers() {
	p.sdk.Register("loop", "turn", p.handleTurn)
	p.sdk.Register("loop", "cancel", p.handleCancel)
	p.sdk.Register("agent", "settings.get", p.handleSettingsGet)
	p.sdk.Register("agent", "settings.save", p.handleSettingsSave)
}

func main() {
	p := createPlugin()
	p.registerHandlers()

	if err := p.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
