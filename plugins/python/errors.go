package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

const (
	codePathInvalid         = "path_invalid"
	codePathNotFound        = "path_not_found"
	codePathIsDirectory     = "path_is_directory"
	codeInterpreterNotFound = "interpreter_not_found"
	codeShellUnavailable    = "shell_unavailable"
)

func errPathInvalid(msg string) error {
	return pluginsdk.ErrCode(codePathInvalid, msg)
}
