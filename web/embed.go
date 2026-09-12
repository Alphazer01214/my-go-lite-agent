package web

import _ "embed"

//go:embed static/shell.html
var shellHTML string

//go:embed static/sdk.js
var sdkJS string

//go:embed static/trace.html
var traceHTML string
