package event

import "context"

type ResearchEventType string

const (
	TypeHeartbeat         ResearchEventType = "HEARTBEAT"
	TypeLog               ResearchEventType = "LOG"
	TypeError             ResearchEventType = "ERROR"
	TypeUserInputReceived ResearchEventType = "USER_INPUT_RECEIVED"
	TypeAnalysisRequested ResearchEventType = "ANALYSIS_REQUESTED"
	TypeSummaryRequested  ResearchEventType = "SUMMARY_REQUESTED"
	TypeSummaryComplete   ResearchEventType = "SUMMARY_COMPLETE"
	TypeTimeout           ResearchEventType = "TIMEOUT"
	TypeTick              ResearchEventType = "TICK"

	TypeSearchRequested     ResearchEventType = "SEARCH_REQUESTED"
	TypeStructuredDataReady ResearchEventType = "STRUCTURED_DATA_READY"
)

type Event struct {
	Type ResearchEventType
	Data any
}

// StatusUpdate represents a transient status message for the frontend.
type StatusUpdate struct {
	Kind    string            `json:"kind"` // always "status"
	Type    ResearchEventType `json:"type"`
	Message string            `json:"message"`
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
