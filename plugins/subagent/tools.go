package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

func definitions() []toolDefinition {
	description := `Delegate one self-contained task to a subagent. The subagent runs in its own session with its own context and the same tools (except this one), and returns its final answer as this tool result. It cannot see this conversation.

## Writing the prompt

Brief the subagent like a smart colleague who just walked into the room — it has not seen this conversation and does not know what you have tried.
- Explain what you are trying to accomplish and why.
- Describe what you have already learned or ruled out.
- Give enough context that it can make judgment calls rather than following a narrow instruction.
- If you need a short response, say so.
- Lookups: hand over the exact command. Investigations: hand over the question; prescribed steps become dead weight when the premise is wrong.

Never delegate understanding. Do not write "based on your findings, fix the bug". Include file paths, line numbers, and the exact change to make.

The subagent starts fresh: the prompt must be self-contained.`

	return []toolDefinition{{
		Name:             "subagent",
		Description:      description,
		Risk:             20,
		ParallelSafe:     true,
		Tags:             []string{"subagent", "delegate"},
		TargetCapability: "subagent",
		TargetMethod:     "run",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"prompt":{"type":"string","description":"A self-contained brief. The subagent cannot see this conversation."},
				"max_steps":{"type":"integer","minimum":1,"maximum":128}
			},
			"required":["prompt"],
			"additionalProperties":false
		}`),
	}}
}

func syncTools(ctx context.Context, sdk sdkCaller) error {
	payload := mustJSON(map[string]any{"tools": definitions()})
	deadline := time.Now().Add(60 * time.Second)
	delay := 50 * time.Millisecond
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := sdk.Call(ctx, "tools", "sync", payload); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tools.sync: %w", lastErr)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 5*time.Second {
			delay *= 2
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
		}
	}
}

func logSyncError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "subagent register tools:", err)
	}
}
