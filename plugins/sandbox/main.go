package main

import (
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func main() {
	cfg, err := loadConfig(configFile)
	if err != nil {
		logf("load config: %v", err)
		cfg = defaultConfig()
	}

	sdk := pluginsdk.NewPlugin("sandbox")
	p := newPlugin(sdk, cfg, configFile)
	p.ui = sdk.WebUI()

	// The consumer set comes from the plugin's own manifest so the handlers
	// registered here always match the Host discovery used by the pipeline.
	for _, seam := range loadManifestSeams("plugin.json") {
		sdk.Register(seam.Capability, seam.Method, p.handleSeam)
	}

	sdk.Register("sandbox", "check", p.handleCheck)
	sdk.Register("sandbox", "pending.list", p.handlePendingList)
	sdk.Register("sandbox", "decide", p.handleDecide)
	sdk.Register("sandbox", "settings.get", p.handleSettingsGet)
	sdk.Register("sandbox", "settings.save", p.handleSettingsSave)

	if err := sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
