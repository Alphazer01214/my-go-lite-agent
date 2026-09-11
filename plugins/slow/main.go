// Command slow is a fixture that delays before answering (for timeout tests).
package main

import (
	"os"
	"time"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func main() {
	for {
		f, err := protocol.ReadFrame(os.Stdin)
		if err != nil {
			return
		}
		if f.Type != protocol.TypeReq {
			continue
		}
		time.Sleep(3 * time.Second)
		_ = protocol.WriteFrame(os.Stdout, &protocol.Frame{
			V: f.V, ID: f.ID, Type: protocol.TypeRes, Cap: f.Cap, Method: f.Method, Payload: f.Payload,
		})
	}
}
