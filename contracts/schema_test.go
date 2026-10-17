package contracts

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type schemaDocument struct {
	Defs map[string]struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	} `json:"$defs"`
}

func TestSchemaMatchesCompatibilityTypes(t *testing.T) {
	data, err := os.ReadFile("core-plugin-contracts.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema schemaDocument
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}

	shapes := map[string]any{
		"usage":             Usage{},
		"functionCall":      FunctionCall{},
		"toolCall":          ToolCall{},
		"toolFunction":      ToolFunction{},
		"toolDefinition":    ToolDefinition{},
		"chatMessage":       ChatMessage{},
		"llmCompleteInput":  LLMCompleteInput{},
		"llmCancelInput":    LLMCancelInput{},
		"llmCancelOutput":   LLMCancelOutput{},
		"llmCompleteOutput": LLMCompleteOutput{},
		"llmTextChunk":      LLMTextChunk{},
		"llmToolCallChunk":  LLMToolCallChunk{},
		"streamMessage":     StreamMessage{},
		"streamSnapshot":    StreamSnapshot{},
		"turnInput":         TurnInput{},
		"turnOutput":        TurnOutput{},
		"cancelInput":       CancelInput{},
		"cancelOutput":      CancelOutput{},
	}

	for name, value := range shapes {
		def, ok := schema.Defs[name]
		if !ok {
			t.Fatalf("schema is missing $defs.%s", name)
		}
		want := jsonFields(reflect.TypeOf(value))
		if len(want) != len(def.Properties) {
			t.Fatalf("%s fields = %v, schema properties = %v", name, want, def.Properties)
		}
		for field := range want {
			if _, ok := def.Properties[field]; !ok {
				t.Fatalf("%s schema is missing property %q", name, field)
			}
		}
		for _, required := range def.Required {
			if !want[required] {
				t.Fatalf("%s requires %q but Go type has no such field", name, required)
			}
		}
	}
}

func jsonFields(typ reflect.Type) map[string]bool {
	fields := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}
