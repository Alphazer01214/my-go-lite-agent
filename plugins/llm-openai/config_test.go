package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaskKey(t *testing.T) {
	if maskKey("") != "" {
		t.Fatal("empty")
	}
	if maskKey("short") != "***" {
		t.Fatalf("short: %q", maskKey("short"))
	}
	got := maskKey("sk-abcdefghij")
	if got != "sk-***hij" {
		t.Fatalf("got %q", got)
	}
}

func TestConfigSetImmediateWhenIdle(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	out, err := p.configSet(configSetIn{Model: "m2", APIKey: "sk-1234567890"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "m2" || out.APIKey != "sk-***890" {
		t.Fatalf("out=%+v", out)
	}
	if p.snapshot().Model != "m2" {
		t.Fatal("not applied")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal("config not written")
	}
}

func TestConfigChangeDoesNotAffectCapturedSnapshot(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	captured := p.snapshot()
	out, err := p.configSet(configSetIn{Model: "busy-model", APIKey: "sk-zzzzzzzzzz"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "busy-model" || out.APIKey != "sk-***zzz" {
		t.Fatalf("out=%+v", out)
	}
	if captured.Model == "busy-model" {
		t.Fatal("captured snapshot must not change")
	}
	if p.snapshot().Model != "busy-model" {
		t.Fatal("subsequent snapshot must use new config")
	}
}

func TestConfigChangesApplyInOrder(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	_, _ = p.configSet(configSetIn{Model: "a"})
	_, _ = p.configSet(configSetIn{Model: "b", APIKey: "sk-zzzzzzzzzz"})
	c := p.snapshot()
	if c.Model != "b" || c.APIKey != "sk-zzzzzzzzzz" {
		t.Fatalf("config=%+v", c)
	}
}
