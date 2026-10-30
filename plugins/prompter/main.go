package main

import (
	"fmt"
	"os"
)

func main() {
	p := createPlugin()
	p.sdk.Register("session", "create", p.handleSessionCreate)
	p.sdk.Register("llm", "stream", p.handleStream)

	if err := p.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
