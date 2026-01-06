package event

type EventType string

const (
	TypeHeartbeat EventType = "HEARTBEAT"
)

type Event struct {
	Type EventType
	Data any
}

