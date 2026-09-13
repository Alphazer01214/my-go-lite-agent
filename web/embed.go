package web

import (
	"embed"
	_ "embed"
)

//go:embed static/shell.html
var shellHTML string

//go:embed static/sdk.js
var sdkJS string

//go:embed static/trace.html
var traceHTML string

// Shell face modules (ADR-0011 ticket 02): the former inline IIFE split
// into native ES modules, served at /app/ with no build tooling.
//
//go:embed static/app
var appFS embed.FS
