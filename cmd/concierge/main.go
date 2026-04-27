package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/user/research-assistant/internal/agent/concierge"
	"github.com/user/research-assistant/internal/config"
	"github.com/user/research-assistant/internal/llm"
	"github.com/user/research-assistant/internal/pubsub"
	"github.com/user/research-assistant/internal/storage"
)

const (
	defaultAddr          = ":8080"
	defaultResearcherURL = "http://localhost:8081"
)

func main() {
	config.LoadEnv()

	addr := config.GetEnv("CONCIERGE_ADDR", defaultAddr)
	researcherURL := config.GetEnv("RESEARCHER_URL", defaultResearcherURL)
	geminiKey := config.GetRequiredEnv("GEMINI_API_KEY")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gemini, err := llm.NewGeminiClient(ctx, geminiKey)
	if err != nil {
		log.Fatalf("[CONCIERGE] Failed to init Gemini client: %v", err)
	}
	defer gemini.Close()

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("[CONCIERGE] Failed to create data dir: %v", err)
	}
	dbStore, err := storage.NewSQLiteStore("data/research.db")
	if err != nil {
		log.Fatalf("[CONCIERGE] Failed to init SQLite store: %v", err)
	}
	defer dbStore.Close()

	// Initialize Redis for pubsub and health check
	redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")
	redisPassword := config.GetEnv("REDIS_PASSWORD", "")
	ps := pubsub.NewRedisPubSub(redisAddr, redisPassword)
	if err := ps.Ping(ctx); err != nil {
		log.Fatalf("[CONCIERGE] Redis health check failed: %v", err)
	}
	log.Printf("[CONCIERGE] Redis health check passed at %s", redisAddr)

	// Build the ResearchStream function that calls the Researcher A2A agent.
	researcherCard := &a2a.AgentCard{
		URL:                researcherURL,
		PreferredTransport: a2a.TransportProtocol("JSONRPC"),
		ProtocolVersion:    "0.2.2",
	}
	researchStream := func(sctx context.Context, topic string) iter.Seq2[a2a.Event, error] {
		return func(yield func(a2a.Event, error) bool) {
			client, err := a2aclient.NewFromCard(sctx, researcherCard)
			if err != nil {
				yield(nil, fmt.Errorf("create researcher client: %w", err))
				return
			}
			msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: topic})
			params := &a2a.MessageSendParams{Message: msg}
			for ev, err := range client.SendStreamingMessage(sctx, params) {
				if !yield(ev, err) {
					return
				}
			}
		}
	}

	exec := concierge.New(gemini, dbStore, researchStream)

	card := &a2a.AgentCard{
		Name:               "Research Assistant — Concierge",
		Description:        "User-facing research agent: accepts research topics, coordinates with the Researcher, relays live status updates, and answers follow-up questions grounded in completed research.",
		URL:                "http://localhost" + addr,
		Version:            "0.1.0",
		ProtocolVersion:    "0.2.2",
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Skills: []a2a.AgentSkill{
			{
				ID:          "research",
				Name:        "Research Topic",
				Description: "Kick off research on a topic and receive live status updates.",
				InputModes:  []string{"text/plain"},
				OutputModes: []string{"application/json"},
			},
			{
				ID:          "qa",
				Name:        "Follow-up Q&A",
				Description: "Answer questions grounded in the completed research session for this context.",
				InputModes:  []string{"text/plain"},
				OutputModes: []string{"text/plain"},
			},
		},
	}

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.Handle("/", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(exec)))
	mux.HandleFunc("/ws", concierge.HandleWebSocket(a2asrv.NewHandler(exec), ps))

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("[CONCIERGE] Listening on %s (researcher: %s)", addr, researcherURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[CONCIERGE] Server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("[CONCIERGE] Shutting down...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[CONCIERGE] Shutdown error: %v", err)
	}
	log.Println("[CONCIERGE] Shutdown complete")
}
