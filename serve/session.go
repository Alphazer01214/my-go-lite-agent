package serve

import (
	"encoding/json"
	"strings"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

// Message is one model-visible chat message.
// Alias of the pluginsdk contract (ADR-0030: types live in pluginsdk).
type Message = pluginsdk.Message

// ToolCall is a model-requested tool invocation.
type ToolCall = pluginsdk.ToolCall

// TurnResult is one Agent Loop turn returned by the mounted loop provider (ADR-0016).
type TurnResult = pluginsdk.TurnResult

// normalizeSessionID maps empty/blank to the default Session id.
func normalizeSessionID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "default"
	}
	return id
}

func messagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
		if a[i].ToolCallID != b[i].ToolCallID {
			return false
		}
		if len(a[i].ToolCalls) != len(b[i].ToolCalls) {
			return false
		}
		for j := range a[i].ToolCalls {
			x, y := a[i].ToolCalls[j], b[i].ToolCalls[j]
			if x.ID != y.ID || x.Name != y.Name || !bytesEqualJSON(x.Arguments, y.Arguments) {
				return false
			}
		}
	}
	return true
}

func bytesEqualJSON(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return string(a) == string(b)
}
