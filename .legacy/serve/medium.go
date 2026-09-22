package serve

import (
	"encoding/json"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/protocol"
)

// PresentationPanelMethod is a PanelOp op (set|clear). See CONTEXT.md PanelOp.
const PresentationPanelMethod = "panel"

// UICap is the Capability for UI Action routing from the Web Shell.
const UICap = "ui"

// UIActionMethod is the method Plugins implement to handle UI Actions.
const UIActionMethod = "action"

// PresentationCap is the Capability used for Presentation Card evt frames.
const PresentationCap = "presentation"

// PresentationCardMethod is the evt method for a Presentation Card.
const PresentationCardMethod = "card"

// PresentationStreamMethod is the ephemeral stream evt (start/chunk/end); not logged.
const PresentationStreamMethod = "stream"

// PresentationStatusMethod is the agent status evt (idle/running) for Render Medium.
const PresentationStatusMethod = "status"

// PresentationRenderMethod is a classified render intent (markdown_text|message_text|summary_text).
const PresentationRenderMethod = "render"

// CommandsCap is the Capability Host uses to route slash commands into a Plugin.
const CommandsCap = "commands"

// CommandsCallMethod is the method Plugins implement to handle a slash command.
const CommandsCallMethod = "call"

// PresentationCard is a structured UI render intent projected from args/result (no I/O).
// Alias of the pluginsdk Card wire type (one definition, ADR-0026).
type PresentationCard = pluginsdk.Card

// Cards returns a copy of Presentation Cards observed this run.
func (s *Server) Cards() []PresentationCard {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PresentationCard, len(s.cards))
	copy(out, s.cards)
	return out
}

func (s *Server) recordCard(f *protocol.Frame) {
	var c PresentationCard
	if len(f.Payload) > 0 {
		if err := json.Unmarshal(f.Payload, &c); err != nil {
			return
		}
	}
	if c.CardType == "" {
		return
	}
	s.mu.Lock()
	s.cards = append(s.cards, c)
	s.mu.Unlock()
}
