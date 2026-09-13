// Command liteagent-server is the Web Render Medium entry: HTTP static
// service, layout, SSE hub, and plugin UI assembly, optionally combined
// with the CLI REPL.
package main

import "github.com/tomori/my-go-lite-agent/internal/app"

func main() { app.Server() }
