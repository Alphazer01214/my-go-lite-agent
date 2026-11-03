package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

const (
	codeCwdInvalid         = "cwd_invalid"
	codeEnvInvalid         = "env_invalid"
	codeCommandNotFound    = "command_not_found"
	codeProcessStartFailed = "process_start_failed"
)

func errCwdInvalid(msg string) error {
	return pluginsdk.ErrCode(codeCwdInvalid, msg)
}

func errEnvInvalid(msg string) error {
	return pluginsdk.ErrCode(codeEnvInvalid, msg)
}
