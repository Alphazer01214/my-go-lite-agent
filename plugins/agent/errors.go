package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
	codeLlmContentFiltered  = "llm_content_filtered"
	codeLlmOutputTruncated  = "llm_output_truncated"
	codeLlmToolCallsMissing = "llm_tool_calls_missing"
)

// Constructors keep the code and the message together at the raise site.
func errLlmContentFiltered(msg string) error {
	return pluginsdk.ErrCode(codeLlmContentFiltered, msg)
}
func errLlmOutputTruncated(msg string) error {
	return pluginsdk.ErrCode(codeLlmOutputTruncated, msg)
}
func errLlmToolCallsMissing(msg string) error {
	return pluginsdk.ErrCode(codeLlmToolCallsMissing, msg)
}
