package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
	codeToolDefinitionInvalid = "tool_definition_invalid"
	codeToolNameConflict      = "tool_name_conflict"
	codeToolNotFound          = "tool_not_found"
	codeToolOwnerMismatch     = "tool_owner_mismatch"
	codeToolSourceInvalid     = "tool_source_invalid"
	codeToolTargetReserved    = "tool_target_reserved"
	codeToolTargetUnavailable = "tool_target_unavailable"
)

// Constructors keep the code and the message together at the raise site.
func errToolDefinitionInvalid(msg string) error {
	return pluginsdk.ErrCode(codeToolDefinitionInvalid, msg)
}
func errToolNameConflict(msg string) error  { return pluginsdk.ErrCode(codeToolNameConflict, msg) }
func errToolNotFound(msg string) error      { return pluginsdk.ErrCode(codeToolNotFound, msg) }
func errToolOwnerMismatch(msg string) error { return pluginsdk.ErrCode(codeToolOwnerMismatch, msg) }
func errToolSourceInvalid(msg string) error { return pluginsdk.ErrCode(codeToolSourceInvalid, msg) }
func errToolTargetReserved(msg string) error {
	return pluginsdk.ErrCode(codeToolTargetReserved, msg)
}
func errToolTargetUnavailable(msg string) error {
	return pluginsdk.ErrCode(codeToolTargetUnavailable, msg)
}
