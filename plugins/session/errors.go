package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// 本插件使用的稳定错误码；平台码（bad_arguments、handler_error）不在此重复。
const (
	codeWorkspaceInvalid  = "workspace_invalid"
	codeWorkspaceMismatch = "workspace_mismatch"
	codeWorkspaceNotFound = "workspace_not_found"
	codeWorkspaceRequired = "workspace_required"
	codeWorkspaceNotEmpty = "workspace_not_empty"
	codeSessionNotFound   = "session_not_found"
)

func errCoded(code, msg string) error {
	return pluginsdk.ErrCode(code, msg)
}
