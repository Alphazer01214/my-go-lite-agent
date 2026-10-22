package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// Error codes this plugin raises. Platform codes (bad_arguments, handler_error)
// and the shared tool result code come from pluginsdk and are not repeated here.
const (
	codeCommandBlocked     = "command_blocked"
	codeCommandNotFound    = "command_not_found"
	codeCwdEscape          = "cwd_escape"
	codeCwdInvalid         = "cwd_invalid"
	codeEnvInvalid         = "env_invalid"
	codeProcessStartFailed = "process_start_failed"
	codeWorkspaceInvalid   = "workspace_invalid"
	codeWorkspaceRequired  = "workspace_required"
)

// Constructors keep the code and the message together at the raise site.
func errCommandBlocked(msg string) error  { return pluginsdk.ErrCode(codeCommandBlocked, msg) }
func errCommandNotFound(msg string) error { return pluginsdk.ErrCode(codeCommandNotFound, msg) }
func errCwdEscape(msg string) error       { return pluginsdk.ErrCode(codeCwdEscape, msg) }
func errCwdInvalid(msg string) error      { return pluginsdk.ErrCode(codeCwdInvalid, msg) }
func errEnvInvalid(msg string) error      { return pluginsdk.ErrCode(codeEnvInvalid, msg) }
func errProcessStartFailed(msg string) error {
	return pluginsdk.ErrCode(codeProcessStartFailed, msg)
}
func errWorkspaceInvalid(msg string) error  { return pluginsdk.ErrCode(codeWorkspaceInvalid, msg) }
func errWorkspaceRequired(msg string) error { return pluginsdk.ErrCode(codeWorkspaceRequired, msg) }
