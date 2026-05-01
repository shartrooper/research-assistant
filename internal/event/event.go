package event

import "context"

type EventType string

const (
	TypeHeartbeat         EventType = "HEARTBEAT"
	TypeLog               EventType = "LOG"
	TypeError             EventType = "ERROR"
	TypeUserInputReceived EventType = "USER_INPUT_RECEIVED"
	TypeAnalysisRequested EventType = "ANALYSIS_REQUESTED"
	TypeSummaryRequested  EventType = "SUMMARY_REQUESTED"
	TypeSummaryComplete   EventType = "SUMMARY_COMPLETE"
	TypeTimeout           EventType = "TIMEOUT"
	TypeTick              EventType = "TICK"

	TypeSearchRequested     EventType = "SEARCH_REQUESTED"
	TypeSearchCompleted     EventType = "SEARCH_COMPLETED"
	TypeStructuredDataReady EventType = "STRUCTURED_DATA_READY"
)

type Event struct {
	Type EventType
	Data any
}

// StatusUpdate represents a transient status message for the frontend.
type StatusUpdate struct {
	Kind    string    `json:"kind"` // always "status"
	Type    EventType `json:"type"`
	Message string    `json:"message"`
}

type SearchSource struct {
	Query   string
	URL     string
	Snippet string
}

type SearchAggregate struct {
	SessionID string
	Topic     string
	Sources   []SearchSource
}

type SummaryPayload struct {
	SessionID  string
	Topic      string
	Summary    string
	Report     string
	Sources    []SearchSource
	Structured StructuredResearch
}

type StructuredFinding struct {
	Finding      string   `json:"finding"`
	EvidenceURLs []string `json:"evidence_urls"`
	Confidence   float64  `json:"confidence"`
}

type StructuredResearch struct {
	SessionID     string              `json:"session_id"`
	Topic         string              `json:"topic"`
	KeyFindings   []StructuredFinding `json:"key_findings"`
	Challenges    []string            `json:"challenges"`
	OpenQuestions []string            `json:"open_questions"`
	Sources       []SearchSource      `json:"sources"`
	Error         string              `json:"error"`
}

type SearchRequest struct {
	SessionID string
	Query     string
}

type SearchResponse struct {
	SessionID string
	Query     string
	Content   string
	URL       string
	Error     error
}

type Publisher interface {
	Publish(Event)
}

// Subscriber defines the interface for subscribing to transient events.
type Subscriber interface {
	SubscribeEvents(ctx context.Context, contextID string) (<-chan Event, error)
}
