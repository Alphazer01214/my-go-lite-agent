package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Author SDK single source: repo sdk/lite-agent.js must match the embedded copy.
func TestSDKCopiesMatch(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "sdk", "lite-agent.js"))
	if err != nil {
		t.Fatal(err)
	}
	embed, err := os.ReadFile(filepath.Join(root, "web", "static", "sdk.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(src) != string(embed) {
		t.Fatal("sdk/lite-agent.js and web/static/sdk.js drifted — rebuild sync")
	}
	for _, name := range []string{"sendMessage", "runCommand"} {
		if !strings.Contains(string(src), name) {
			t.Fatalf("SDK missing %s", name)
		}
	}
}
