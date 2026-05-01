package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/user/research-assistant/internal/agent/researcher"
	"github.com/user/research-assistant/internal/config"
	"github.com/user/research-assistant/internal/llm"
	"github.com/user/research-assistant/internal/pipeline"
	"github.com/user/research-assistant/internal/pubsub"
	"github.com/user/research-assistant/internal/search"
	"github.com/user/research-assistant/internal/storage"
)

const defaultAddr = ":8081"

func main() {
	config.LoadEnv()

	addr := config.GetEnv("RESEARCHER_ADDR", defaultAddr)
	geminiKey := config.GetRequiredEnv("GEMINI_API_KEY")
	cseKey := config.GetEnv("CSE_API_KEY", "")
	cseCx := config.GetEnv("CSE_CX", "")

	// Redis for real-time events
	redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")
	redisPassword := config.GetEnv("REDIS_PASSWORD", "")
	ps := pubsub.NewRedisPubSub(redisAddr, redisPassword)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gemini, err := llm.NewGeminiClient(ctx, geminiKey)
	if err != nil {
		log.Fatalf("[RESEARCHER] Failed to init Gemini client: %v", err)
	}
	defer gemini.Close()

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("[RESEARCHER] Failed to create data dir: %v", err)
	}
	dbStore, err := storage.NewSQLiteStore("data/research.db")
	if err != nil {
		log.Fatalf("[RESEARCHER] Failed to init SQLite store: %v", err)
	}
	defer dbStore.Close()

	blobStore, err := storage.NewDiskBlobStore("artifacts")
	if err != nil {
		log.Fatalf("[RESEARCHER] Failed to init blob store: %v", err)
	}

	searchFn := func(sctx context.Context, query string) ([]pipeline.SearchResult, error) {
		if cseKey == "" || cseCx == "" {
			return nil, fmt.Errorf("CSE not configured (set CSE_API_KEY and CSE_CX)")
		}
		items, err := search.ContentWebSearch(sctx, cseKey, cseCx, query, search.Options{Num: 3})
		if err != nil {
			return nil, err
		}
		out := make([]pipeline.SearchResult, 0, len(items))
		for _, r := range items {
			var sb strings.Builder
			if r.Snippet != "" {
				sb.WriteString(r.Snippet)
			}
			out = append(out, pipeline.SearchResult{Content: sb.String(), URL: r.Link})
		}
		return out, nil
	}

	pl := pipeline.New(gemini, searchFn, dbStore, blobStore)
	exec := researcher.New(pl, ps)

	card := &a2a.AgentCard{
		Name:               "Research Assistant — Researcher",
		Description:        "Runs the full research pipeline for a given topic: query generation, parallel web search, LLM structuring, report writing, and artifact persistence.",
		URL:                "http://localhost" + addr,
		Version:            "0.1.0",
		ProtocolVersion:    "0.2.2",
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"application/json"},
		Skills: []a2a.AgentSkill{
			{
				ID:          "research",
				Name:        "Research Topic",
				Description: "Given a research topic, produces a structured report with key findings, sources, and an executive summary.",
				InputModes:  []string{"text/plain"},
				OutputModes: []string{"application/json"},
			},
		},
	}

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.Handle("/", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(exec)))

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("[RESEARCHER] Listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[RESEARCHER] Server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("[RESEARCHER] Shutting down...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[RESEARCHER] Shutdown error: %v", err)
	}
	log.Println("[RESEARCHER] Shutdown complete")
}
