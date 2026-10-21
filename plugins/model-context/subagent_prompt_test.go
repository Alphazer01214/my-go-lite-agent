package main

import (
	"strings"
	"testing"
)

func TestAssembleAppendsConfiguredSubagentPrompt(t *testing.T) {
	p := newPlugin(nil)
	asm, err := p.assemble(completeMeta{SessionID: "s1"}, deriveOut{}, "subagent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asm.SystemPrompt, "You are a subagent delegated one self-contained task.") {
		t.Fatal("subagent prompt section was not appended")
	}
	found := false
	for _, section := range asm.SystemSections {
		if section.Name == "subagent" {
			found = true
			if section.Order != 1000 || section.Hash == "" {
				t.Fatalf("subagent audit = %#v", section)
			}
		}
	}
	if !found {
		t.Fatal("missing subagent section audit")
	}
}

func TestAssembleOmitsSubagentPromptForMainSession(t *testing.T) {
	p := newPlugin(nil)
	asm, err := p.assemble(completeMeta{SessionID: "s1"}, deriveOut{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(asm.SystemPrompt, "You are a subagent delegated one self-contained task.") {
		t.Fatal("main session received the subagent prompt")
	}
}
