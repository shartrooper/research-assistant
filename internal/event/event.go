package event

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

	TypeSearchRequested EventType = "SEARCH_REQUESTED"
	TypeSearchCompleted EventType = "SEARCH_COMPLETED"
)

type Event struct {
	Type EventType
	Data any
}

type SearchSource struct {
	Query   string
	URL     string
	Snippet string
}

type SearchAggregate struct {
	Topic   string
	Sources []SearchSource
}

type SummaryPayload struct {
	Topic   string
	Summary string
	Report  string
	Sources []SearchSource
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
