package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/user/research-assistant/internal/config"
	"github.com/user/research-assistant/internal/engine"
	"github.com/user/research-assistant/internal/event"
	"github.com/user/research-assistant/internal/llm"
	"github.com/user/research-assistant/internal/search"
)

func main() {
	fmt.Println("Starting Research Assistant...")

	// Load environment variables using the centralized utility
	config.LoadEnv()

	geminiKey := config.GetRequiredEnv("GEMINI_API_KEY")
	cseKey := config.GetEnv("CSE_API_KEY", "")
	cseCx := config.GetEnv("CSE_CX", "")

	// Create a root context for the application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Gemini Client
	gemini, err := llm.NewGeminiClient(ctx, geminiKey)
	if err != nil {
		log.Fatalf("Failed to initialize Gemini client: %v", err)
	}
	defer gemini.Close()

	// Initialize Session Manager
	sm := engine.NewSessionManager()

	// Phase 4: Parallel Workers & Gemini API Protection
	// 3 workers, but max 2 concurrent analyses
	en := engine.New(32, 5, 2, func(ev event.Event, p event.Publisher) {
		switch ev.Type {
		case event.TypeTick:
			// System heartbeat - low noise
			// fmt.Printf("[TICK] %v\n", ev.Data)

		case event.TypeUserInputReceived:
			fmt.Printf("[1. INPUT] Processing prompt: %v\n", ev.Data)

			go func() {
				// Generate search queries
				prompt := fmt.Sprintf(`Given the following research topic, generate 3 specific search queries to gather comprehensive information. Return ONLY a JSON array of strings.
					Topic: %v Return ONLY the JSON.`, ev.Data)

				resp, err := gemini.GenerateContent(ctx, prompt)
				if err != nil {
					fmt.Printf("[! ERROR] Failed to generate search queries: %v\n", err)
					return
				}

				// Basic JSON extraction (Gemini often wraps in code blocks)
				jsonStr := resp
				if idx := strings.Index(jsonStr, "["); idx != -1 {
					jsonStr = jsonStr[idx:]
				}
				if idx := strings.LastIndex(jsonStr, "]"); idx != -1 {
					jsonStr = jsonStr[:idx+1]
				}

				var queries []string
				if err := json.Unmarshal([]byte(jsonStr), &queries); err != nil {
					fmt.Printf("[! ERROR] Failed to parse search queries: %v. Raw: %s\n", err, resp)
					// Fallback to a single search
					queries = []string{fmt.Sprintf("%v", ev.Data)}
				}

				sessionID := uuid.New().String()
				sm.CreateSession(sessionID, fmt.Sprintf("%v", ev.Data), len(queries))

				fmt.Printf("[SESSION %s] Initiating %d searches...\n", sessionID[:8], len(queries))

				for _, q := range queries {
					p.Publish(event.Event{
						Type: event.TypeSearchRequested,
						Data: event.SearchRequest{
							SessionID: sessionID,
							Query:     q,
						},
					})
				}
			}()

		case event.TypeSearchRequested:
			req := ev.Data.(event.SearchRequest)
			fmt.Printf("[2. SEARCH] Query: %s\n", req.Query)

			go func() {
				searchCtx, searchCancel := context.WithTimeout(ctx, 30*time.Second)
				defer searchCancel()

				response := event.SearchResponse{
					SessionID: req.SessionID,
					Query:     req.Query,
				}

				// Use Google CSE (required)
				if cseKey == "" || cseCx == "" {
					response.Error = fmt.Errorf("CSE not configured (set CSE_API_KEY and CSE_CX)")
				} else {
					results, err := search.SearchWeb(searchCtx, cseKey, cseCx, req.Query, search.Options{Num: 3})
					if err != nil {
						response.Error = err
					} else if len(results) > 0 {
						var contentBuilder strings.Builder
						var links []string
						for i, r := range results {
							if r.Snippet != "" {
								contentBuilder.WriteString(fmt.Sprintf("(%d) %s\n", i+1, r.Snippet))
							}
							if r.Link != "" {
								links = append(links, r.Link)
							}
						}
						response.Content = contentBuilder.String()
						response.URL = strings.Join(links, " | ")
					}
				}

				p.Publish(event.Event{
					Type: event.TypeSearchCompleted,
					Data: response,
				})
			}()

		case event.TypeSearchCompleted:
			res := ev.Data.(event.SearchResponse)
			session, ok := sm.GetSession(res.SessionID)
			if !ok {
				return
			}

			if res.Error != nil {
				fmt.Printf("[! ERROR] Search failed for '%s': %v\n", res.Query, res.Error)
			} else {
				fmt.Printf("[SEARCH OK] Result from %s\n", res.URL)
			}

			if session.AddResult(res) {
				fmt.Printf("[SESSION %s] All searches complete. Starting analysis...\n", session.ID[:8])

				// Aggregate context
				var contextBuilder strings.Builder
				contextBuilder.WriteString(fmt.Sprintf("Original Topic: %s\n\n", session.OriginalPrompt))
				contextBuilder.WriteString("Research Findings:\n")
				for _, r := range session.Results {
					if r.Error == nil && r.Content != "" {
						contextBuilder.WriteString(fmt.Sprintf("- Source: %s\n  Content: %s\n\n", r.URL, r.Content))
					}
				}

				p.Publish(event.Event{
					Type: event.TypeAnalysisRequested,
					Data: contextBuilder.String(),
				})

				// Clean up session
				sm.DeleteSession(session.ID)
			}

		case event.TypeAnalysisRequested:
			contextData := ev.Data.(string)
			fmt.Printf("[3. ANALYSIS] Synthesizing research data...\n")

			// Run the analysis using Gemini asynchronously
			go func() {
				analysisCtx, analysisCancel := context.WithTimeout(ctx, 90*time.Second)
				defer analysisCancel()

				prompt := fmt.Sprintf(`You are a research assistant. Based on the provided search results, create a comprehensive report on the topic. 
Include key insights, challenges, and a conclusion.
Search Data:
%s`, contextData)

				result, err := gemini.GenerateContent(analysisCtx, prompt)
				if err != nil {
					fmt.Printf("[! ERROR] Gemini analysis failed: %v\n", err)
					p.Publish(event.Event{
						Type: event.TypeError,
						Data: err.Error(),
					})
					return
				}

				p.Publish(event.Event{
					Type: event.TypeSummaryRequested,
					Data: result,
				})
			}()

		case event.TypeSummaryRequested:
			fmt.Printf("[4. SUMMARY] Generating final report...\n")
			summary := fmt.Sprintf("RESEARCH REPORT\n===============\n%v", ev.Data)
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
