package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

// 本插件使用的稳定错误码；平台码（bad_arguments、handler_error）不在此重复。
const (
	codeWorkspaceInvalid  = "workspace_invalid"
	codeWorkspaceNotFound = "workspace_not_found"
	codeWorkspaceRequired = "workspace_required"
	codeWorkspaceNotEmpty = "workspace_not_empty"
	codeSessionNotFound   = "session_not_found"
)

func errCoded(code, msg string) error {
	return pluginsdk.ErrCode(code, msg)
}

// 错误消息约定：除稳定的 error_code 外，msg 必须给出可执行的恢复指引
// （调用方接下来该调哪个方法），让 agent 无需猜测即可自行恢复。

// errSessionNotFound 会话不存在；指引调用方先用 session.create 创建。
func errSessionNotFound(id string) error {
	return errCoded(codeSessionNotFound, "session not found: "+id+"; call session.create first")
}

// errWorkspaceNotFound workspace 不存在；指引调用方改用 workspace.create 或 workspace.list。
func errWorkspaceNotFound(path string) error {
	return errCoded(codeWorkspaceNotFound, "workspace not found: "+path+"; call workspace.create or workspace.list to pick a valid workspace path")
}
