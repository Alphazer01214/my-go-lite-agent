package main

import (
	"fmt"
	"os"
)

func main() {
	p := createPlugin()
	p.sdk.Register("loop", "turn", p.handleTurn)
	p.sdk.Register("loop", "cancel", p.handleCancel)

	if err := p.sdk.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
