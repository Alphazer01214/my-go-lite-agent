package main

import "encoding/json"

var subagentParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "prompt": {
      "type": "string",
      "minLength": 1,
      "description": "A self-contained brief. The subagent cannot see this conversation."
    }
  },
  "required": ["prompt"],
  "additionalProperties": false
}`)

const subagentDescription = `Delegate one self-contained task to a subagent. The subagent runs in its own child session with the same workspace, model, agent loop and tools, and returns its final answer as this tool result.

## Writing the prompt

Brief the subagent like a smart colleague who just walked into the room — it has not seen this conversation and does not know what you have tried.
- Explain what you are trying to accomplish and why.
- Describe what you have already learned or ruled out.
- Give enough context that it can make judgment calls rather than following a narrow instruction.
- If you need a short response, say so.
- Lookups: hand over the exact command. Investigations: hand over the question; prescribed steps become dead weight when the premise is wrong.

Never delegate understanding. Do not write "based on your findings, fix the bug". Include file paths, line numbers, and the exact change to make.

The subagent starts fresh: the prompt must be self-contained. A subagent cannot delegate beyond the configured delegation depth.

Multiple subagent calls in one step run in parallel. Only delegate independent tasks: parallel subagents share the same workspace and may race if they edit the same files.`

func (p *plugin) toolInfo() infoResponse {
	return infoResponse{
		Capability: "subagent",
		Tools: []infoTool{{
			Name:        "subagent",
			Description: subagentDescription,
			Parameters:  subagentParameters,
			Method:      "run",
		}},
	}
}
