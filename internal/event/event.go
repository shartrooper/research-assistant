package event

type EventType string

const (
	TypeHeartbeat EventType = "HEARTBEAT"
	TypeLog       EventType = "LOG"
	TypeError     EventType = "ERROR"
)

type Event struct {
	Type EventType
	Data any
}

type Publisher interface {
	Publish(Event)
}
