package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/research-assistant/internal/event"
)

var eventQueue = make(chan event.Event, 32)

func main() {
	fmt.Println("Starting Research Assistant (Phase 0)...")

	// Start the main event loop in a goroutine
	go mainLoop()

	// Initial heartbeat event to prove the loop works
	eventQueue <- event.Event{
		Type: event.TypeHeartbeat,
		Data: time.Now(),
	}

	// Wait for termination signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("\nShutting down...")
}

func mainLoop() {
	for ev := range eventQueue {
		dispatch(ev)
	}
}

func dispatch(ev event.Event) {
	switch ev.Type {
	case event.TypeHeartbeat:
		fmt.Printf("[EVENT] %s: System heartbeat at %v\n", ev.Type, ev.Data)
	default:
		fmt.Printf("[EVENT] Unknown event type: %s\n", ev.Type)
	}
}
