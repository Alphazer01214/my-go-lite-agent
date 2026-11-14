package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestSaveLoadAgentConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config{
		MaxSteps:          42,
		ToolParallelism:   toolParallelismAuto,
		MaxParallelTools:  defaultMaxParallelTools,
		ParallelSafeTools: append([]string(nil), defaultParallelSafeTools...),
	}
	if err := saveConfig(path, want); err != nil {
		t.Fatalf("saveConfig: %v", err)
	}
	got, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestAgentSettingsHandlersAndRegistrations(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldDir)

	p := &plugin{sdk: pluginsdk.NewPlugin("agent"), cfg: defaultConfig(), turns: make(map[string]*activeTurn)}
	p.registerHandlers()
	registered := map[string]bool{}
	for _, item := range p.sdk.ListRegistered() {
		registered[item.Capability+"."+item.Method] = true
	}
	for _, key := range []string{"agent.settings.get", "agent.settings.save"} {
		if !registered[key] {
			t.Fatalf("missing registration %s", key)
		}
	}

	raw, err := p.handleSettingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"max_steps":42}}`),
	})
	if err != nil {
		t.Fatalf("settings.save: %v", err)
	}
	var saved settingsSaveResp
	if err := json.Unmarshal(raw, &saved); err != nil || !saved.Saved {
		t.Fatalf("save response = %s err = %v", raw, err)
	}
	raw, err = p.handleSettingsGet(context.Background(), &pluginsdk.Request{})
	if err != nil {
		t.Fatalf("settings.get: %v", err)
	}
	var got agentSettingsGetResp
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Values.MaxSteps != 42 {
		t.Fatalf("max_steps = %d, want 42", got.Values.MaxSteps)
	}
	if got.Values.ToolParallelism != toolParallelismAuto {
		t.Fatalf("tool_parallelism = %q, want %q", got.Values.ToolParallelism, toolParallelismAuto)
	}
	if got.Values.MaxParallelTools != defaultMaxParallelTools {
		t.Fatalf("max_parallel_tools = %d, want %d", got.Values.MaxParallelTools, defaultMaxParallelTools)
	}

	raw, err = p.handleSettingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"max_steps":42,"tool_parallelism":"serial","max_parallel_tools":2,"parallel_safe_tools":["file_read","subagent"]}}`),
	})
	if err != nil {
		t.Fatalf("settings.save parallel fields: %v", err)
	}
	raw, err = p.handleSettingsGet(context.Background(), &pluginsdk.Request{})
	if err != nil {
		t.Fatalf("settings.get: %v", err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Values.ToolParallelism != toolParallelismSerial || got.Values.MaxParallelTools != 2 {
		t.Fatalf("parallel settings = %+v", got.Values)
	}
	if !reflect.DeepEqual(got.Values.ParallelSafeTools, []string{"file_read", "subagent"}) {
		t.Fatalf("parallel_safe_tools = %v", got.Values.ParallelSafeTools)
	}
}

func TestAgentSettingsRejectsNonPositiveMaxSteps(t *testing.T) {
	p := &plugin{cfg: defaultConfig(), turns: make(map[string]*activeTurn)}
	_, err := p.handleSettingsSave(context.Background(), &pluginsdk.Request{
		Payload: json.RawMessage(`{"values":{"max_steps":0}}`),
	})
	if err == nil {
		t.Fatal("expected invalid max_steps error")
	}
}
