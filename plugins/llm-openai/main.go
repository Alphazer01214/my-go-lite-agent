package main

import (
	"fmt"
	"os"
)

func (p *plugin) registerHandlers() {
	p.sdk.Register("llm", "stream", p.handleStream)
	p.sdk.Register("llm", "settings.get", p.handleSettingsGet)
	p.sdk.Register("llm", "settings.save", p.handleSettingsSave)
	p.sdk.Register("llm", "models.list", p.handleModelsList)
}

func main() {
	p := createPlugin()
	p.registerHandlers()

	if err := p.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
