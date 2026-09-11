// Command consumer is a fixture Plugin: it provides demo and calls echo only through Host.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tomori/my-go-lite-agent/protocol"
)

func main() {
	pending := map[string]string{} // outbound id -> invoke id

	for {
		f, err := protocol.ReadFrame(os.Stdin)
		if err != nil {
			return
		}
		switch f.Type {
		case protocol.TypeReq:
			// Host asked us to invoke another Capability (default: echo).
			// Ticket 04: routing target is chosen by Host via payload.cap, else echo.
			capName := "echo"
			var body struct {
				Cap string `json:"cap"`
			}
			if len(f.Payload) > 0 {
				_ = json.Unmarshal(f.Payload, &body)
				if body.Cap != "" {
					capName = body.Cap
				}
			}
			oid := "out-" + f.ID
			pending[oid] = f.ID
			out := &protocol.Frame{
				V:       1,
				ID:      oid,
				Type:    protocol.TypeReq,
				Cap:     capName,
				Method:  "echo",
				Payload: json.RawMessage(`{"via":"host"}`),
			}
			if err := protocol.WriteFrame(os.Stdout, out); err != nil {
				return
			}
		case protocol.TypeRes:
			orig, ok := pending[f.ID]
			if !ok {
				continue
			}
			delete(pending, f.ID)
			res := &protocol.Frame{
				V:      1,
				ID:     orig,
				Type:   protocol.TypeRes,
				Cap:    "demo",
				Method: "invoke",
			}
			if f.Error != nil {
				res.Error = f.Error
			} else {
				res.Payload = f.Payload
			}
			if err := protocol.WriteFrame(os.Stdout, res); err != nil {
				return
			}
		default:
			fmt.Fprintf(os.Stderr, "consumer: ignore type=%s\n", f.Type)
		}
	}
}
