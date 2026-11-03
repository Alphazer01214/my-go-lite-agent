package main

import "github.com/tomori/my-go-lite-agent/pluginsdk"

const (
	codeSkillNotFound    = "skill_not_found"
	codeSkillUnavailable = "skill_unavailable"
	codeSkillReadFailed  = "skill_read_failed"

	codeMCPConfigInvalid = "mcp_config_invalid"
	codeMCPUnavailable   = "mcp_server_unavailable"
	codeMCPCallFailed    = "mcp_call_failed"
)

func errSkillNotFound(name string) error {
	return pluginsdk.ErrCode(codeSkillNotFound, "skill not found: "+name)
}

func errSkillUnavailable(name string) error {
	return pluginsdk.ErrCode(codeSkillUnavailable, "skill has no readable SKILL.md: "+name)
}
