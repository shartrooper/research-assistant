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
	"github.com/user/research-assistant/internal/artifacts"
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

				var sources []event.SearchSource
				for _, r := range session.Results {
					if r.Error == nil && r.Content != "" {
						sources = append(sources, event.SearchSource{
							Query:   r.Query,
							URL:     r.URL,
							Snippet: r.Content,
						})
					}
				}

				p.Publish(event.Event{
					Type: event.TypeAnalysisRequested,
					Data: event.SearchAggregate{
						Topic:   session.OriginalPrompt,
						Sources: sources,
					},
				})

				// Clean up session
				sm.DeleteSession(session.ID)
			}

		case event.TypeAnalysisRequested:
			agg := ev.Data.(event.SearchAggregate)
			fmt.Printf("[3. ANALYSIS] Synthesizing research data...\n")

			// Run the analysis using Gemini asynchronously
			go func() {
				analysisCtx, analysisCancel := context.WithTimeout(ctx, 90*time.Second)
				defer analysisCancel()

				var sourceBuilder strings.Builder
				for _, s := range agg.Sources {
					sourceBuilder.WriteString(fmt.Sprintf("- Source: %s\n  Query: %s\n  Snippet: %s\n\n", s.URL, s.Query, s.Snippet))
				}

				structPrompt := fmt.Sprintf(`You are a research assistant. Convert the search results into the following JSON schema.
Return ONLY valid JSON. No commentary. No markdown.

Schema:
{
  "topic": "string",
  "key_findings": [
    {
      "finding": "string",
      "evidence_urls": ["string"],
      "confidence": 0.0
    }
  ],
  "challenges": ["string"],
  "open_questions": ["string"],
  "sources": [
    {
      "url": "string",
      "query": "string",
      "snippet": "string"
    }
  ],
  "error": "string"
}

Rules:
- Use only the provided sources.
- evidence_urls must be URLs from sources.
- confidence ranges 0.0–1.0.
- If unsure, reduce confidence and add an open question.
- If the topic is gibberish, unsafe, disallowed, or cannot be meaningfully researched, set "error" to a short explanation and return empty arrays for key_findings/challenges/open_questions/sources.
- Unsafe/disallowed examples: making bombs, domestic terrorism, suicide assistance, or any other harmful instructions.

Topic: %s

Sources:
%s`, agg.Topic, sourceBuilder.String())

				rawStructured, err := gemini.GenerateContent(analysisCtx, structPrompt)
				structured := event.StructuredResearch{
					Topic:         agg.Topic,
					Sources:       agg.Sources,
					OpenQuestions: []string{"Structured extraction failed; using raw sources."},
					Error:         "",
				}
				if err != nil {
					fmt.Printf("[! ERROR] Gemini structuring failed: %v\n", err)
				} else if parsed, parseErr := parseStructuredResearch(rawStructured); parseErr == nil {
					structured = parsed
				} else {
					fmt.Printf("[! ERROR] JSON parse failed: %v\n", parseErr)
				}

				p.Publish(event.Event{
					Type: event.TypeStructuredDataReady,
					Data: structured,
				})
			}()

		case event.TypeStructuredDataReady:
			structured := ev.Data.(event.StructuredResearch)
			fmt.Printf("[3. STRUCTURED] Organizing findings...\n")

			go func() {
				analysisCtx, analysisCancel := context.WithTimeout(ctx, 90*time.Second)
				defer analysisCancel()

				if strings.TrimSpace(structured.Error) != "" {
					message := fmt.Sprintf("Unable to perform research for this topic: %s", structured.Error)
					p.Publish(event.Event{
						Type: event.TypeSummaryRequested,
						Data: event.SummaryPayload{
							Topic:      structured.Topic,
							Summary:    message,
							Report:     message,
							Sources:    structured.Sources,
							Structured: structured,
						},
					})
					return
				}

				structuredJSON, _ := json.MarshalIndent(structured, "", "  ")
				reportPrompt := fmt.Sprintf(`You are a research assistant. Write a comprehensive report based only on the structured data below.
Include key insights, challenges, and a conclusion.
Structured Data:
%s`, string(structuredJSON))

				report, err := gemini.GenerateContent(analysisCtx, reportPrompt)
				if err != nil {
					fmt.Printf("[! ERROR] Gemini analysis failed: %v\n", err)
					p.Publish(event.Event{
						Type: event.TypeError,
						Data: err.Error(),
					})
					return
				}

				summaryPrompt := fmt.Sprintf(`Create a short executive summary (3-5 bullet points) for the following report.
Return plain text bullets.
Report:
%s`, report)

				execSummary, err := gemini.GenerateContent(analysisCtx, summaryPrompt)
				if err != nil {
					fmt.Printf("[! ERROR] Gemini summary failed: %v\n", err)
					execSummary = "Executive summary unavailable due to generation error."
				}

				p.Publish(event.Event{
					Type: event.TypeSummaryRequested,
					Data: event.SummaryPayload{
						Topic:      structured.Topic,
						Summary:    execSummary,
						Report:     report,
						Sources:    structured.Sources,
						Structured: structured,
					},
				})
			}()

		case event.TypeSummaryRequested:
			fmt.Printf("[4. SUMMARY] Generating final report...\n")
			payload := ev.Data.(event.SummaryPayload)
			summary := fmt.Sprintf("RESEARCH REPORT\n===============\n%s", payload.Report)
			p.Publish(event.Event{
				Type: event.TypeSummaryComplete,
				Data: event.SummaryPayload{
					Topic:      payload.Topic,
					Summary:    payload.Summary,
					Report:     summary,
					Sources:    payload.Sources,
					Structured: payload.Structured,
				},
			})

		case event.TypeSummaryComplete:
			payload := ev.Data.(event.SummaryPayload)
			fmt.Printf("[4. COMPLETE] Output ready.\n")

			dir, err := artifacts.WriteBundle("artifacts", artifacts.Bundle{
				Topic:      payload.Topic,
				Summary:    payload.Summary,
				Report:     payload.Report,
				Sources:    payload.Sources,
				Structured: payload.Structured,
			})
			if err != nil {
				fmt.Printf("[! ERROR] Failed to write artifacts: %v\n", err)
				return
			}
			fmt.Printf("[ARTIFACTS] Written to %s\n", dir)

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

func parseStructuredResearch(raw string) (event.StructuredResearch, error) {
	cleaned := strings.TrimSpace(raw)
	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start == -1 || end == -1 || end <= start {
		return event.StructuredResearch{}, fmt.Errorf("no json object found")
	}

	cleaned = cleaned[start : end+1]
	var sr event.StructuredResearch
	if err := json.Unmarshal([]byte(cleaned), &sr); err != nil {
		return event.StructuredResearch{}, err
	}
	if sr.Topic == "" {
		sr.Topic = "Unknown Topic"
	}
	if sr.Error != "" {
		sr.KeyFindings = nil
		sr.Challenges = nil
		sr.OpenQuestions = nil
		sr.Sources = nil
	}
	validateStructuredResearch(&sr)
	return sr, nil
}

func validateStructuredResearch(sr *event.StructuredResearch) {
	sourceSet := make(map[string]struct{}, len(sr.Sources))
	for i := range sr.Sources {
		s := &sr.Sources[i]
		s.URL = strings.TrimSpace(s.URL)
		s.Query = strings.TrimSpace(s.Query)
		s.Snippet = strings.TrimSpace(s.Snippet)
		if s.URL != "" {
			sourceSet[s.URL] = struct{}{}
		}
	}

	for i := range sr.KeyFindings {
		f := &sr.KeyFindings[i]
		f.Finding = strings.TrimSpace(f.Finding)
		if f.Confidence < 0 {
			f.Confidence = 0
		}
		if f.Confidence > 1 {
			f.Confidence = 1
		}
		if len(sourceSet) == 0 {
			continue
		}
		filtered := make([]string, 0, len(f.EvidenceURLs))
		for _, u := range f.EvidenceURLs {
			u = strings.TrimSpace(u)
			if _, ok := sourceSet[u]; ok {
				filtered = append(filtered, u)
			}
		}
		f.EvidenceURLs = filtered
	}
}
