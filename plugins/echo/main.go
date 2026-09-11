// Command echo is a fixture Plugin: every req becomes a res with the same id.
package main

import (
	"os"

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
		res := &protocol.Frame{
			V:       f.V,
			ID:      f.ID,
			Type:    protocol.TypeRes,
			Cap:     f.Cap,
			Method:  f.Method,
			Payload: f.Payload,
		}
		if err := protocol.WriteFrame(os.Stdout, res); err != nil {
			return
		}
	}
}
