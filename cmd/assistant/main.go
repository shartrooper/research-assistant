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
		case event.TypeUserInputReceived:
			fmt.Printf("[1. INPUT] Processing prompt: %v\n", ev.Data)
			p.Publish(event.Event{
				Type: event.TypeAnalysisRequested,
				Data: ev.Data,
			})

		case event.TypeAnalysisRequested:
			fmt.Printf("[2. ANALYSIS] Analyzing data for: %v\n", ev.Data)
			// Simulating enrichment: adding "analyzed" context
			analysisResult := fmt.Sprintf("Deep analysis of '%v'", ev.Data)
			p.Publish(event.Event{
				Type: event.TypeSummaryRequested,
				Data: analysisResult,
			})

		case event.TypeSummaryRequested:
			fmt.Printf("[3. SUMMARY] Summarizing results: %v\n", ev.Data)
			summary := fmt.Sprintf("SUMMARY: This is the final report on [%v]", ev.Data)
			p.Publish(event.Event{
				Type: event.TypeSummaryComplete,
				Data: summary,
			})

		case event.TypeSummaryComplete:
			fmt.Printf("[4. COMPLETE] Output ready: %v\n", ev.Data)

		case event.TypeHeartbeat:
			fmt.Printf("[SYSTEM] %s: Heartbeat at %v\n", ev.Type, ev.Data)

		default:
			fmt.Printf("[SYSTEM] Unknown event type: %s\n", ev.Type)
		}
	})

	en.Start()

	// Phase 2: Start the 3-step pipeline
	en.Publish(event.Event{
		Type: event.TypeUserInputReceived,
		Data: "The future of Go concurrency",
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")
	en.Stop()
	fmt.Println("Shutdown complete")
}
