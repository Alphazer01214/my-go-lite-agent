package main

import (
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	sdk := pluginsdk.NewPlugin("subagent")
	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "subagent: load config: %v\n", err)
		cfg = defaultConfig()
	}
	p := newPlugin(sdk, cfg)

	sdk.Register("subagent", "info", p.handleInfo)
	sdk.Register("subagent", "run", p.handleRun)
	sdk.Register("subagent", "settings.get", p.handleSettingsGet)
	sdk.Register("subagent", "settings.save", p.handleSettingsSave)

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
