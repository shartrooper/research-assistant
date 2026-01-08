package engine

import (
	"fmt"
	"sync"

	"github.com/user/research-assistant/internal/event"
)

type Handler func(event.Event, event.Publisher)

type State string

const (
	StateIdle        State = "IDLE"
	StateAnalyzing   State = "ANALYZING"
	StateSummarizing State = "SUMMARIZING"
)

type Engine struct {
	eventQueue chan event.Event
	wg         sync.WaitGroup
	handler    Handler
	state      State
}

func New(bufferSize int, handler Handler) *Engine {
	return &Engine{
		eventQueue: make(chan event.Event, bufferSize),
		handler:    handler,
		state:      StateIdle,
	}
}

// background event loop

func (e *Engine) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for ev := range e.eventQueue {
			e.dispatch(ev)
		}
	}()
}

// Send an event to the queue
func (e *Engine) Publish(ev event.Event) {
	e.eventQueue <- ev
}

// closing the queue and wait for all events to be processed
func (e *Engine) Stop() {
	close(e.eventQueue)
	e.wg.Wait()
}

func (e *Engine) dispatch(ev event.Event) {
	// --- THE BOUNCER (FSM) ---
	switch e.state {
	case StateIdle:
		if ev.Type != event.TypeUserInputReceived && ev.Type != event.TypeHeartbeat {
			fmt.Printf("[BOUNCER] Ignored %s: System is IDLE. Please start with User Input.\n", ev.Type)
			return
		}
		if ev.Type == event.TypeUserInputReceived {
			e.state = StateAnalyzing
		}

	case StateAnalyzing:
		if ev.Type == event.TypeUserInputReceived {
			fmt.Printf("[BOUNCER] Rejected %s: I am already ANALYZING something else!\n", ev.Type)
			return
		}
		if ev.Type == event.TypeSummaryRequested {
			e.state = StateSummarizing
		}

	case StateSummarizing:
		if ev.Type == event.TypeUserInputReceived || ev.Type == event.TypeAnalysisRequested {
			fmt.Printf("[BOUNCER] Rejected %s: I am busy SUMMARIZING!\n", ev.Type)
			return
		}
		if ev.Type == event.TypeSummaryComplete {
			e.state = StateIdle
		}
	}

	if e.handler != nil {
		e.handler(ev, e)
		return
	}

	switch ev.Type {
	case event.TypeHeartbeat:
		fmt.Printf("[ENGINE] %s: System heartbeat at %v\n", ev.Type, ev.Data)
	default:
		fmt.Printf("[ENGINE] Unknown event type: %s\n", ev.Type)
	}
}
