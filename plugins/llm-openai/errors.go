package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
	codeLlmUpstream = "llm_upstream"
)

// Constructors keep the code and the message together at the raise site.
func errLlmUpstream(msg string) error { return pluginsdk.ErrCode(codeLlmUpstream, msg) }
