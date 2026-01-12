package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/research-assistant/internal/config"
	"github.com/user/research-assistant/internal/engine"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/llm"
)

func main() {
	fmt.Println("Starting Research Assistant...")

	// Load environment variables using the centralized utility
	config.LoadEnv()

	apiKey := config.GetRequiredEnv("GEMINI_API_KEY")

	// Create a root context for the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Gemini Client
	gemini, err := llm.NewGeminiClient(ctx, apiKey)
	if err != nil {
		log.Fatalf("Failed to initialize Gemini client: %v", err)
	}
	defer gemini.Close()

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
			fmt.Printf("[2. ANALYSIS] Analyzing data using Gemini for: %v\n", ev.Data)

			// Run the analysis using Gemini asynchronously
			go func() {
				// PHASE 5: Context-based timeout
				// We give each analysis a 90-second timeout for real API calls
				// We create the context INSIDE the goroutine so it's not cancelled prematurely
				analysisCtx, analysisCancel := context.WithTimeout(ctx, 90*time.Second)
				defer analysisCancel()

				prompt := fmt.Sprintf("Analyze the following research topic and provide key insights: %v", ev.Data)
				result, err := gemini.GenerateContent(analysisCtx, prompt)
				if err != nil {
					fmt.Printf("[! ERROR] Gemini analysis failed for '%v': %v\n", ev.Data, err)
					p.Publish(event.Event{
						Type: event.TypeError,
						Data: err.Error(),
					})
					// Also trigger timeout recovery to free up capacity
					p.Publish(event.Event{
						Type: event.TypeTimeout,
						Data: ev.Data,
					})
					return
				}

				p.Publish(event.Event{
					Type: event.TypeSummaryRequested,
					Data: result,
				})
			}()

		case event.TypeSummaryRequested:
			fmt.Printf("[3. SUMMARY] Summarizing results: %v\n", ev.Data)
			// For now, we just pass through the Gemini result or wrap it
			summary := fmt.Sprintf("SUMMARY REPORT:\n%v", ev.Data)
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
