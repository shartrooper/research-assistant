package event

type EventType string

const (
	TypeHeartbeat         EventType = "HEARTBEAT"
	TypeLog               EventType = "LOG"
	TypeError             EventType = "ERROR"
	TypeAnalysisRequested EventType = "ANALYSIS_REQUESTED"
	TypeAnalysisComplete  EventType = "ANALYSIS_COMPLETE"
)

type Event struct {
	Type EventType
	Data any
}

type Publisher interface {
	Publish(Event)
}
