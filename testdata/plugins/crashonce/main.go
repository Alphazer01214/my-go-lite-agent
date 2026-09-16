// Command crashonce exits on first process start, then acts like echo (for on-demand restart).
package main

import (
	"os"
	"path/filepath"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func main() {
	flag := filepath.Join(filepath.Dir(os.Args[0]), "crash-once.flag")
	if _, err := os.Stat(flag); err != nil {
		_ = os.WriteFile(flag, []byte("1"), 0o644)
		os.Exit(1)
	}
	for {
		f, err := protocol.ReadFrame(os.Stdin)
		if err != nil {
			return
		}
		if f.Type != protocol.TypeReq {
			continue
		}
		_ = protocol.WriteFrame(os.Stdout, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method, Payload: f.Payload,
		})
	}
}
