package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	eventQueue    chan event.Event
	wg            sync.WaitGroup
	handler       Handler
	numWorkers    int
	maxConcurrent int

	mu             sync.Mutex
	activeAnalyses int
	stopChan       chan struct{}
}

func New(bufferSize int, numWorkers int, maxConcurrent int, handler Handler) *Engine {
	return &Engine{
		eventQueue:     make(chan event.Event, bufferSize),
		handler:        handler,
		numWorkers:     numWorkers,
		maxConcurrent:  maxConcurrent,
		activeAnalyses: 0,
		stopChan:       make(chan struct{}),
	}
}

// Start launches the worker pool
func (e *Engine) Start(ctx context.Context) {
	// 1. Start the worker pool
	for i := 0; i < e.numWorkers; i++ {
		e.wg.Add(1)
		go func(workerID int) {
			defer e.wg.Done()
			for ev := range e.eventQueue {
				e.dispatch(ev, workerID)
			}
		}(i)
	}

	// 2. Start the system heartbeat ticker
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				e.Publish(event.Event{Type: event.TypeTick, Data: time.Now()})
			case <-ctx.Done():
				return
			}
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

func (e *Engine) dispatch(ev event.Event, workerID int) {
	// --- THE CONCURRENCY BOUNCER ---
	if ev.Type == event.TypeUserInputReceived {
		e.mu.Lock()
		if e.activeAnalyses >= e.maxConcurrent {
			fmt.Printf("[WORKER %d] Rejected %s: Capacity reached (%d/%d). Gemini API protection active.\n",
				workerID, ev.Type, e.activeAnalyses, e.maxConcurrent)
			e.mu.Unlock()
			return
		}
		e.activeAnalyses++
		fmt.Printf("[WORKER %d] Accepted %s: Capacity (%d/%d)\n",
			workerID, ev.Type, e.activeAnalyses, e.maxConcurrent)
		e.mu.Unlock()
	}

	// Logic to decrement counter when a pipeline finishes or times out
	if ev.Type == event.TypeSummaryComplete || ev.Type == event.TypeTimeout {
		e.mu.Lock()
		e.activeAnalyses--
		if e.activeAnalyses < 0 {
			e.activeAnalyses = 0
		}
		reason := "Finished task"
		if ev.Type == event.TypeTimeout {
			reason = "Task timed out"
		}
		fmt.Printf("[WORKER %d] %s. Capacity now (%d/%d)\n", 
			workerID, reason, e.activeAnalyses, e.maxConcurrent)
		e.mu.Unlock()
	}

	if e.handler != nil {
		e.handler(ev, e)
		return
	}

	switch ev.Type {
	case event.TypeHeartbeat:
		fmt.Printf("[WORKER %d] %s: Heartbeat at %v\n", workerID, ev.Type, ev.Data)
	default:
		fmt.Printf("[WORKER %d] Unknown event type: %s\n", workerID, ev.Type)
	}
}
