package pluginsdk

import "encoding/json"

// Presentation wire contract (breaking changes require protocol field bump).
//
// Cards are broadcast evt Frames: cap=presentation, method=card, no id.
// Render intents are broadcast evt Frames: cap=presentation, method=render.
//
// Protocol v2 render kinds: markdown_text | message_text | summary_text.
const (
	PresentationCap       = "presentation"
	PresentationMethod    = "card"
	PresentationRender    = "render"
	PresentationPanel     = "panel"
	PresentationStreamEvt = "stream"
	PresentationStatusEvt = "status"
	CommandsCap           = "commands"
	CommandsCallMethod    = "call"
	UICap                 = "ui"
	UICallMethod          = "action"
)

// PanelOp is one Web Medium panel mutation (set|clear). See CONTEXT.md PanelOp.
type PanelOp struct {
	Op        string          `json:"op"`                  // set | clear
	Slot      string          `json:"slot"`                // sidebar | main-overlay | toolbar-right
	ID        string          `json:"id"`                  // stable panel id; set replaces by id
	Component string          `json:"component,omitempty"` // custom element tag "<plugin>-*", required for set
	Props     json.RawMessage `json:"props,omitempty"`     // JSON object passed to the element
}

// EmitPanel sends a Panel Component mutation (set|clear) as a broadcast evt
// Frame. Host validates component prefixing before fanning out (ADR-0010).
func (s *Server) EmitPanel(op PanelOp) error {
	payload, err := json.Marshal(op)
	if err != nil {
		return err
	}
	return s.Emit(PresentationCap, PresentationPanel, payload)
}

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
	RenderMarkdownText RenderKind = "markdown_text"
	RenderMessageText  RenderKind = "message_text"
	RenderSummaryText  RenderKind = "summary_text"
)

// SummaryPair is one ordered key/value row on a summary_text card.
type SummaryPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RenderIntent is one classified block for the terminal (or other) Render Medium.
type RenderIntent struct {
	Kind RenderKind `json:"kind"`
	// Markdown body (kind=markdown_text).
	Text string `json:"text,omitempty"`
	// Short status line (kind=message_text): info | warn | error | dim.
	Level string `json:"level,omitempty"`
	// Key/value summary (kind=summary_text).
	Title  string        `json:"title,omitempty"`
	Pairs  []SummaryPair `json:"pairs,omitempty"`
	Detail string        `json:"detail,omitempty"`
}

// EmitRender sends a classified render intent (broadcast).
func (s *Server) EmitRender(ri RenderIntent) error {
	payload, err := json.Marshal(ri)
	if err != nil {
		return err
	}
	return s.Emit(PresentationCap, PresentationRender, payload)
}

// EmitMarkdownText is shorthand for a markdown_text body block.
func (s *Server) EmitMarkdownText(text string) error {
	return s.EmitRender(RenderIntent{Kind: RenderMarkdownText, Text: text})
}

// EmitMessageText is shorthand for a message_text status line.
func (s *Server) EmitMessageText(level, text string) error {
	return s.EmitRender(RenderIntent{Kind: RenderMessageText, Level: level, Text: text})
}

// EmitSummaryText is shorthand for a summary_text card.
func (s *Server) EmitSummaryText(title string, pairs []SummaryPair, detail string) error {
	return s.EmitRender(RenderIntent{Kind: RenderSummaryText, Title: title, Pairs: pairs, Detail: detail})
}
