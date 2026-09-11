package pluginsdk

import "encoding/json"

// Presentation wire contract (breaking changes require protocol field bump).
//
// Cards are broadcast evt Frames: cap=presentation, method=card, no id.
// Render intents are broadcast evt Frames: cap=presentation, method=render.
const (
	PresentationCap       = "presentation"
	PresentationMethod    = "card"
	PresentationRender    = "render"
	PresentationStreamEvt = "stream"
	PresentationStatusEvt = "status"
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

// RenderKind classifies main-window content for the Render Medium.
type RenderKind string

const (
	RenderMarkdown   RenderKind = "markdown"
	RenderExpandable RenderKind = "expandable"
	RenderMessage    RenderKind = "message"
)

// RenderIntent is one classified block for the terminal (or other) Render Medium.
type RenderIntent struct {
	Kind RenderKind `json:"kind"`
	// Markdown body (kind=markdown).
	Text string `json:"text,omitempty"`
	// Expandable section (kind=expandable).
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	Detail string `json:"detail,omitempty"`
	Open   bool   `json:"open,omitempty"`
	// Plain status line (kind=message).
	Level string `json:"level,omitempty"` // info | warn | error
}

// EmitRender sends a classified render intent (broadcast).
func (s *Server) EmitRender(ri RenderIntent) error {
	payload, err := json.Marshal(ri)
	if err != nil {
		return err
	}
	return s.Emit(PresentationCap, PresentationRender, payload)
}

// EmitMarkdown is shorthand for a markdown body block.
func (s *Server) EmitMarkdown(text string) error {
	return s.EmitRender(RenderIntent{Kind: RenderMarkdown, Text: text})
}

// EmitExpandable is shorthand for a collapsible section.
func (s *Server) EmitExpandable(title, body, detail string, open bool) error {
	return s.EmitRender(RenderIntent{
		Kind: RenderExpandable, Title: title, Body: body, Detail: detail, Open: open,
	})
}

// EmitMessage is shorthand for a plain status/error line.
func (s *Server) EmitMessage(level, text string) error {
	return s.EmitRender(RenderIntent{Kind: RenderMessage, Level: level, Text: text})
}
