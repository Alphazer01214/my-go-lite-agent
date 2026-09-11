// Command asyncsubllm is a fixture LLM: first hop requests run_subagent mode=async;
// later hops return a final reply.
package main

import (
	"encoding/json"
	"sync/atomic"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

var hops atomic.Int32

func main() {
	s := pluginsdk.New()
	s.Handle("llm", "complete", func(req *pluginsdk.Request) (json.RawMessage, error) {
		n := hops.Add(1)
		if n == 1 {
			args, _ := json.Marshal(map[string]string{
				"input": "async-child",
				"mode":  "async",
			})
			out, _ := json.Marshal(map[string]any{
				"content": "",
				"tool_calls": []map[string]any{{
					"id":        "call-async",
					"name":      "run_subagent",
					"arguments": json.RawMessage(args),
				}},
			})
			return out, nil
		}
		out, _ := json.Marshal(map[string]any{"content": "async path done"})
		return out, nil
	})
	_ = s.Serve()
}
