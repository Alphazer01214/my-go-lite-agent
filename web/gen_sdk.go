//go:build ignore

// Command gen_sdk is the SDK single-source generator (ADR / sdk):
// copies sdk/lite-agent.js into the embedded web/static/sdk.js copy.
// Run from web/:  go generate ./...
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root, err := filepath.Abs(filepath.Join(".."))
	if err != nil {
		fmt.Fprintln(os.Stderr, "abs:", err)
		os.Exit(1)
	}
	src := filepath.Join(root, "sdk", "lite-agent.js")
	dst := filepath.Join(root, "web", "static", "sdk.js")
	raw, err := os.ReadFile(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read sdk:", err)
		os.Exit(1)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write sdk:", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		fmt.Fprintln(os.Stderr, "rename sdk:", err)
		os.Exit(1)
	}
	fmt.Printf("sdk: copied sdk/lite-agent.js -> web/static/sdk.js (%d bytes)\n", len(raw))
}