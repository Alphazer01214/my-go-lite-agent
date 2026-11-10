package main

import (
	"fmt"
	"os"
)

func main() {
	p := createPlugin()
	p.sdk.Register("git", "status", p.handleStatus)
	p.sdk.Register("git", "branches", p.handleBranches)
	p.sdk.Register("git", "diff", p.handleDiff)
	p.sdk.Register("git", "checkout", p.handleCheckout)
	p.sdk.Register("git", "branch.create", p.handleBranchCreate)
	p.sdk.Register("git", "branch.delete", p.handleBranchDelete)

	if err := p.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
