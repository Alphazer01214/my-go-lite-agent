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

func TestConfigSetDeferredWhileWorking(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	p.beginWork()
	out, err := p.configSet(configSetIn{Model: "busy-model"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "busy-model" {
		t.Fatalf("should report pending target, got %+v", out)
	}
	if p.snapshot().Model == "busy-model" {
		t.Fatal("must not apply while working")
	}
	p.endWork()
	if p.snapshot().Model != "busy-model" {
		t.Fatalf("must flush on idle, got %+v", p.snapshot())
	}
}

func TestConfigSetMergeWhileWorking(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(old) }()

	p := newPlugin()
	p.beginWork()
	_, _ = p.configSet(configSetIn{Model: "a"})
	_, _ = p.configSet(configSetIn{Model: "b", APIKey: "sk-zzzzzzzzzz"})
	p.endWork()
	c := p.snapshot()
	if c.Model != "b" || c.APIKey != "sk-zzzzzzzzzz" {
		t.Fatalf("merged: %+v", c)
	}
}
