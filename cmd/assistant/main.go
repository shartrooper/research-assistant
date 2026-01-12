package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/research-assistant/internal/engine"
	"github.com/user/research-assistant/internal/event"
)

func main() {
	fmt.Println("Starting Research Assistant...")

	// Create a root context for the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Phase 4: Parallel Workers & Gemini API Protection
	// 3 workers, but max 2 concurrent analyses
	en := engine.New(32, 3, 2, func(ev event.Event, p event.Publisher) {
		switch ev.Type {
		case event.TypeTick:
			// System heartbeat - low noise
			// fmt.Printf("[TICK] %v\n", ev.Data)

		case event.TypeUserInputReceived:
			fmt.Printf("[1. INPUT] Processing prompt: %v\n", ev.Data)
			p.Publish(event.Event{
				Type: event.TypeAnalysisRequested,
				Data: ev.Data,
			})

		case event.TypeAnalysisRequested:
			fmt.Printf("[2. ANALYSIS] Analyzing data for: %v\n", ev.Data)

			// PHASE 5: Context-based timeout
			// We give each analysis a 1-second timeout
			// (But our mock process takes 2 seconds)
			analysisCtx, analysisCancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer analysisCancel()

			// Run the analysis in a way that respects context
			done := make(chan string)
			go func() {
				time.Sleep(2 * time.Second) // Mock work
				done <- fmt.Sprintf("Deep analysis of '%v'", ev.Data)
			}()

			select {
			case result := <-done:
				p.Publish(event.Event{
					Type: event.TypeSummaryRequested,
					Data: result,
				})
			case <-analysisCtx.Done():
				fmt.Printf("[! TIMEOUT] Analysis for '%v' exceeded time limit\n", ev.Data)
				p.Publish(event.Event{
					Type: event.TypeTimeout,
					Data: ev.Data,
				})
			}

		case event.TypeSummaryRequested:
			fmt.Printf("[3. SUMMARY] Summarizing results: %v\n", ev.Data)
			summary := fmt.Sprintf("SUMMARY: This is the final report on [%v]", ev.Data)
			p.Publish(event.Event{
				Type: event.TypeSummaryComplete,
				Data: summary,
			})

		case event.TypeSummaryComplete:
			fmt.Printf("[4. COMPLETE] Output ready: %v\n", ev.Data)

		case event.TypeTimeout:
			fmt.Printf("[RECOVERY] Cleaning up after timeout for: %v\n", ev.Data)

		case event.TypeHeartbeat:
			fmt.Printf("[SYSTEM] %s: Heartbeat at %v\n", ev.Type, ev.Data)

		default:
			// fmt.Printf("[SYSTEM] Unknown event type: %s\n", ev.Type)
		}
	})

	en.Start(ctx)

	// Phase 4: Stress Test - Send 4 prompts
	// With maxConcurrent=2, prompts 3 and 4 should be rejected
	prompts := []string{
		"The future of Go concurrency",
		"How to build event-driven systems",
		"Gemini API rate limiting strategies",
		"Why are worker pools useful?",
	}

	for _, p := range prompts {
		en.Publish(event.Event{
			Type: event.TypeUserInputReceived,
			Data: p,
		})
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("Shutting down...")
	en.Stop()
	fmt.Println("Shutdown complete")
}
