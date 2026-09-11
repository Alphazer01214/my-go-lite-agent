package pluginsdk

import "encoding/json"

// Presentation wire contract (breaking changes require protocol field bump).
//
// Cards are broadcast evt Frames: cap=presentation, method=card, no id.
// Payload is a Card. Projection must be a pure function of args/result/meta —
// no I/O, clock, or randomness — so replay yields the same Card.
const (
	PresentationCap    = "presentation"
	PresentationMethod = "card"
)

// Card is one Presentation render intent (CONTEXT.md Presentation Card).
type Card struct {
	CardType string          `json:"cardType"`
	Tool     string          `json:"tool,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// EmitCard sends a Presentation Card as a broadcast evt Frame.
func (s *Server) EmitCard(c Card) error {
	payload, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.Emit(PresentationCap, PresentationMethod, payload)
}
