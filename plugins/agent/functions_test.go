package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
)

func TestTitleFromInput(t *testing.T) {
	if got := titleFromInput("  你好\n世界  "); got != "你好 世界" {
		t.Fatalf("title = %q", got)
	}
	long := strings.Repeat("字", 35)
	got := titleFromInput(long)
	runes := []rune(got)
	if len(runes) != 31 || runes[len(runes)-1] != '…' {
		t.Fatalf("long title = %q (%d runes)", got, len(runes))
	}
	if got := titleFromInput("   \n\t "); got != "New Session" {
		t.Fatalf("empty title = %q", got)
	}
}

func TestIsCancelled(t *testing.T) {
	if !isCancelled(context.Canceled) {
		t.Fatal("context.Canceled should count as cancelled")
	}
	if !isCancelled(pluginsdk.ErrCode(pluginsdk.CodeCallCancelled, "cancelled")) {
		t.Fatal("call_cancelled should count as cancelled")
	}
	if isCancelled(errors.New("boom")) {
		t.Fatal("plain error must not count as cancelled")
	}
	if isCancelled(nil) {
		t.Fatal("nil must not count as cancelled")
	}
}
