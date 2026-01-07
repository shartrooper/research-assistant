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
)

type Event struct {
	Type EventType
	Data any
}

type Publisher interface {
	Publish(Event)
}
