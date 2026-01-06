package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/user/research-assistant/internal/engine"
	"github.com/user/research-assistant/internal/event"
)

func main() {
	fmt.Println("Starting Research Assistant...")

	en := engine.New(32, func(ev event.Event, p event.Publisher) {
		switch ev.Type {
		case event.TypeHeartbeat:
			fmt.Printf("[MAIN] %s: System heartbeat at %v\n", ev.Type, ev.Data)
		case event.TypeAnalysisRequested:
			fmt.Printf("[MAIN] %s: Starting analysis for: %v\n", ev.Type, ev.Data)
			p.Publish(event.Event{
				Type: event.TypeAnalysisComplete,
				Data: fmt.Sprintf("Results for %v", ev.Data),
			})
		case event.TypeAnalysisComplete:
			fmt.Printf("[MAIN] %s: Analysis finished: %v\n", ev.Type, ev.Data)
		default:
			fmt.Printf("[MAIN] Unknown event type: %s\n", ev.Type)
		}
	})

	en.Start()

	// Initial chain trigger
	en.Publish(event.Event{
		Type: event.TypeAnalysisRequested,
		Data: "Phase 1 Research",
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")
	en.Stop()
	fmt.Println("Shutdown complete")
}
