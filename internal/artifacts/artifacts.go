package artifacts

import (
	"github.com/user/research-assistant/internal/event"
)

type Bundle struct {
	Topic      string                   `json:"topic"`
	Summary    string                   `json:"summary"`
	Report     string                   `json:"report"`
	Sources    []event.SearchSource     `json:"sources"`
	Structured event.StructuredResearch `json:"structured"`
}
